package admin

import (
	"context"
	"unicode/utf8"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"go.uber.org/zap"
)

// ChunkService 文档块管理服务接口。
type ChunkService interface {
	ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.ChunkVO, int64, error)
	ListByDoc(ctx context.Context, docID string, q model.PageQuery) ([]model.ChunkVO, int64, error)
	Get(ctx context.Context, id string) (*model.ChunkVO, error)
	Update(ctx context.Context, id string, req model.ChunkUpdateReq) error
	Toggle(ctx context.Context, id string) (enabled int, err error)
}

type chunkService struct {
	repo repository.ChunkRepository
}

// NewChunkService 创建 Chunk 服务。
func NewChunkService(repo repository.ChunkRepository) ChunkService {
	return &chunkService{repo: repo}
}

func (s *chunkService) ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.ChunkVO, int64, error) {
	dos, total, err := s.repo.ListByKB(ctx, kbID, q)
	if err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.ChunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.ChunkDOToVO(d))
	}
	return vos, total, nil
}

func (s *chunkService) ListByDoc(ctx context.Context, docID string, q model.PageQuery) ([]model.ChunkVO, int64, error) {
	dos, total, err := s.repo.ListByDoc(ctx, docID, q)
	if err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.ChunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.ChunkDOToVO(d))
	}
	return vos, total, nil
}

func (s *chunkService) Get(ctx context.Context, id string) (*model.ChunkVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Chunk 不存在")
	}
	vo := model.ChunkDOToVO(*do)
	return &vo, nil
}

func (s *chunkService) Update(ctx context.Context, id string, req model.ChunkUpdateReq) error {
	updates := map[string]any{}
	if req.Text != nil {
		updates["text"] = *req.Text
		updates["char_count"] = utf8.RuneCountInString(*req.Text)
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新 Chunk 失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	return nil
}

func (s *chunkService) Toggle(ctx context.Context, id string) (int, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return 0, errs.NotFound("Chunk 不存在")
	}
	newEnabled := 0
	if do.Enabled == 0 {
		newEnabled = 1
	}
	if err := s.repo.UpdateFields(ctx, id, map[string]any{"enabled": newEnabled}); err != nil {
		zap.L().Error("切换 Chunk 状态失败", zap.Error(err))
		return 0, errs.WrapBusiness(err, "操作失败")
	}
	return newEnabled, nil
}
