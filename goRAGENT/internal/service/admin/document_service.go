package admin

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
	"go.uber.org/zap"
)

// Ingestor 入库触发抽象。DocumentService.Upload 末尾调用 Run 异步启动入库流水线。
type Ingestor interface {
	Run(taskID int64)
}

// DocumentService 文档管理服务接口。
type DocumentService interface {
	ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.DocumentVO, int64, error)
	Search(ctx context.Context, keyword string, q model.PageQuery) ([]model.DocumentVO, int64, error)
	Get(ctx context.Context, id string) (*model.DocumentVO, error)
	Upload(ctx context.Context, kbID, fileName string, reader io.Reader) (*model.DocumentVO, error)
	Preview(ctx context.Context, docID string) (content string, docName string, err error)
	Download(ctx context.Context, docID string) (filePath, fileName string, err error)
	Delete(ctx context.Context, id string) error
	Toggle(ctx context.Context, docID string) (enabled int, err error)
}

type documentService struct {
	docRepo  repository.DocumentRepository
	chunkRepo repository.ChunkRepository
	taskRepo repository.IngestionTaskRepository
	kbRepo   repository.KnowledgeBaseRepository
	ingestor Ingestor
	dataDir  string
}

// NewDocumentService 创建文档服务。
func NewDocumentService(
	docRepo repository.DocumentRepository,
	chunkRepo repository.ChunkRepository,
	taskRepo repository.IngestionTaskRepository,
	kbRepo repository.KnowledgeBaseRepository,
	ingestor Ingestor,
	dataDir string,
) DocumentService {
	return &documentService{
		docRepo: docRepo, chunkRepo: chunkRepo, taskRepo: taskRepo,
		kbRepo: kbRepo, ingestor: ingestor, dataDir: dataDir,
	}
}

func (s *documentService) ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.DocumentVO, int64, error) {
	dos, total, err := s.docRepo.ListByKB(ctx, kbID, q)
	if err != nil {
		zap.L().Error("查询文档列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.DocumentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.DocumentDOToVO(d))
	}
	return vos, total, nil
}

func (s *documentService) Search(ctx context.Context, keyword string, q model.PageQuery) ([]model.DocumentVO, int64, error) {
	dos, total, err := s.docRepo.Search(ctx, keyword, q)
	if err != nil {
		zap.L().Error("搜索文档失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "搜索失败")
	}
	vos := make([]model.DocumentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.DocumentDOToVO(d))
	}
	return vos, total, nil
}

func (s *documentService) Get(ctx context.Context, id string) (*model.DocumentVO, error) {
	do, err := s.docRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("文档不存在")
	}
	vo := model.DocumentDOToVO(*do)
	return &vo, nil
}

func (s *documentService) Upload(ctx context.Context, kbID, fileName string, reader io.Reader) (*model.DocumentVO, error) {
	// 1. 验证知识库存在
	kb, err := s.kbRepo.FindByID(ctx, kbID)
	if err != nil {
		return nil, errs.Business("知识库不存在")
	}
	_ = kb

	// 2. 生成文档 ID + 保存文件
	docID := snowflake.NextID()
	ext := filepath.Ext(fileName)
	fileDir := filepath.Join(s.dataDir, "files", kbID)
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		zap.L().Error("创建文件目录失败", zap.String("dir", fileDir), zap.Error(err))
		return nil, errs.WrapServer(err, "文件保存失败")
	}

	destPath := filepath.Join(fileDir, docID+ext)
	dst, err := os.Create(destPath)
	if err != nil {
		zap.L().Error("创建文件失败", zap.String("path", destPath), zap.Error(err))
		return nil, errs.WrapServer(err, "文件保存失败")
	}
	defer dst.Close()

	written, err := io.Copy(dst, reader)
	if err != nil {
		zap.L().Error("写入文件失败", zap.String("path", destPath), zap.Error(err))
		os.Remove(destPath)
		return nil, errs.WrapServer(err, "文件保存失败")
	}
	if err := dst.Sync(); err != nil {
		zap.L().Error("同步文件失败", zap.String("path", destPath), zap.Error(err))
		os.Remove(destPath)
		return nil, errs.WrapServer(err, "文件保存失败")
	}

	// 3. 创建文档记录
	doc := model.DocumentDO{
		ID: docID, KbID: kbID, FileName: fileName,
		FileType: ext, FileSize: written, Status: model.DocStatusPending,
	}
	if err := s.docRepo.Create(ctx, &doc); err != nil {
		zap.L().Error("创建文档记录失败", zap.Error(err))
		os.Remove(destPath)
		return nil, errs.WrapBusiness(err, "创建文档失败")
	}

	// 4. 创建入库任务
	task := model.IngestionTaskDO{
		KbID: kbID, DocID: docID, Status: model.TaskStatusPending,
	}
	if err := s.taskRepo.Create(ctx, &task); err != nil {
		zap.L().Error("创建入库任务失败", zap.Error(err))
		// 回滚：删除已写文件 + 删除文档记录
		os.Remove(destPath)
		_ = s.docRepo.SoftDelete(ctx, docID)
		return nil, errs.WrapBusiness(err, "创建入库任务失败")
	}

	// 5. 触发入库引擎（异步）
	if s.ingestor != nil {
		s.ingestor.Run(task.ID)
	}

	vo := model.DocumentDOToVO(doc)
	return &vo, nil
}

func (s *documentService) Preview(ctx context.Context, docID string) (string, string, error) {
	do, err := s.docRepo.FindByID(ctx, docID)
	if err != nil {
		return "", "", errs.NotFound("文档不存在")
	}

	parsedPath := filepath.Join(s.dataDir, "parsed", do.ID+".md")
	data, err := os.ReadFile(parsedPath)
	if err != nil {
		return "", "", errs.Business("解析产物尚未生成")
	}

	content := string(data)
	runes := []rune(content)
	if len(runes) > model.DocumentPreviewMaxRunes {
		content = string(runes[:model.DocumentPreviewMaxRunes]) + "\n\n... (内容过长，已截断)"
	}

	return content, do.FileName, nil
}

func (s *documentService) Download(ctx context.Context, docID string) (string, string, error) {
	do, err := s.docRepo.FindByID(ctx, docID)
	if err != nil {
		return "", "", errs.NotFound("文档不存在")
	}

	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	filePath := filepath.Join(s.dataDir, "files", do.KbID, do.ID+ext)

	return filePath, do.FileName, nil
}

func (s *documentService) Delete(ctx context.Context, id string) error {
	do, err := s.docRepo.FindByID(ctx, id)
	if err != nil {
		return errs.NotFound("文档不存在")
	}

	// 先软删文档（确保文档存在才往下走）
	if err := s.docRepo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除文档失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}

	// 级联软删 chunk（修复：处理之前被忽略的 error）
	if err := s.chunkRepo.SoftDeleteByDoc(ctx, id); err != nil {
		zap.L().Error("级联删除 Chunk 失败", zap.Error(err))
		// 文档已删，chunk 删除失败仅记录日志，不阻断
	}

	// 清理磁盘文件
	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	os.Remove(filepath.Join(s.dataDir, "files", do.KbID, id+ext))
	os.Remove(filepath.Join(s.dataDir, "parsed", id+".md"))

	return nil
}

func (s *documentService) Toggle(ctx context.Context, docID string) (int, error) {
	// 查找任一 chunk 确定当前 enabled 状态
	chunks, _, err := s.chunkRepo.ListByDoc(ctx, docID, model.PageQuery{Page: 1, Size: 1})
	if err != nil {
		zap.L().Error("查询 Chunk 状态失败", zap.Error(err))
		return 0, errs.WrapServer(err, "操作失败")
	}

	currentEnabled := 0
	if len(chunks) > 0 {
		currentEnabled = chunks[0].Enabled
	}

	newEnabled := 0
	if currentEnabled == 0 {
		newEnabled = 1
	}

	if err := s.chunkRepo.UpdateFieldsByDoc(ctx, docID, map[string]any{"enabled": newEnabled}); err != nil {
		zap.L().Error("切换文档启用状态失败", zap.Error(err))
		return 0, errs.WrapBusiness(err, "操作失败")
	}

	return newEnabled, nil
}
