package admin

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/snowflake"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type documentVO struct {
	ID         string `json:"id"`
	KbID       string `json:"kbId"`
	FileName   string `json:"fileName"`
	FileType   string `json:"fileType"`
	FileSize   int64  `json:"fileSize"`
	Status     string `json:"status"`
	ChunkCount int    `json:"chunkCount"`
	CreateTime string `json:"createTime"`
}

func docDOtoVO(d rag.DocumentDO) documentVO {
	return documentVO{
		ID: d.ID, KbID: d.KbID, FileName: d.FileName, FileType: d.FileType,
		FileSize: d.FileSize, Status: d.Status, ChunkCount: d.ChunkCount,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}

func (h *Handler) listDocuments(c *gin.Context) {
	kbID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.DocumentDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).
		Where("kb_id = ? AND deleted = 0", kbID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("kb_id = ? AND deleted = 0", kbID).
		Order("create_time DESC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询文档列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]documentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, docDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) uploadDocument(c *gin.Context) {
	kbID := c.Param("id")

	var kb rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", kbID).First(&kb).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "请选择文件"))
		return
	}
	defer file.Close()

	docID := snowflake.NextID()
	ext := filepath.Ext(header.Filename)

	fileDir := filepath.Join(h.dataDir, "files", kbID)
	os.MkdirAll(fileDir, 0755)

	destPath := filepath.Join(fileDir, docID+ext)
	dst, err := os.Create(destPath)
	if err != nil {
		zap.L().Error("创建文件失败", zap.String("path", destPath), zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "文件保存失败"))
		return
	}
	defer dst.Close()

	written, _ := io.Copy(dst, file)

	doc := rag.DocumentDO{
		ID: docID, KbID: kbID, FileName: header.Filename,
		FileType: ext, FileSize: written, Status: rag.DocStatusPending,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&doc).Error; err != nil {
		zap.L().Error("创建文档记录失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建文档失败"))
		return
	}

	task := rag.IngestionTaskDO{
		KbID: kbID, DocID: docID, Status: rag.TaskStatusPending,
	}
	h.db.WithContext(c.Request.Context()).Create(&task)

	if h.ingestionEngine != nil {
		h.ingestionEngine.Run(task.ID)
	}

	c.JSON(http.StatusOK, response.Success(docDOtoVO(doc)))
}

func (h *Handler) searchDocuments(c *gin.Context) {
	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.DocumentDO
	var total int64
	query := h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("deleted = 0")
	if keyword != "" {
		query = query.Where("file_name LIKE ?", "%"+keyword+"%")
	}
	query.Count(&total)
	if err := query.Order("create_time DESC").Offset((page-1)*pageSize).Limit(pageSize).Find(&dos).Error; err != nil {
		zap.L().Error("搜索文档失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "搜索失败"))
		return
	}

	vos := make([]documentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, docDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(docDOtoVO(do)))
}

func (h *Handler) previewDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	parsedPath := filepath.Join(h.dataDir, "parsed", do.ID+".md")
	data, err := os.ReadFile(parsedPath)
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "解析产物尚未生成"))
		return
	}

	content := string(data)
	runes := []rune(content)
	if len(runes) > 5000 {
		content = string(runes[:5000]) + "\n\n... (内容过长，已截断)"
	}

	c.JSON(http.StatusOK, response.Success(gin.H{"content": content, "docName": do.FileName}))
}

func (h *Handler) downloadDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	filePath := filepath.Join(h.dataDir, "files", do.KbID, do.ID+ext)

	c.Header("Content-Disposition", "attachment; filename=\""+do.FileName+"\"")
	c.File(filePath)
}

func (h *Handler) deleteDocument(c *gin.Context) {
	id := c.Param("id")

	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", id).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("id = ?", id).Update("deleted", 1)
	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).Where("doc_id = ?", id).Update("deleted", 1)

	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	os.Remove(filepath.Join(h.dataDir, "files", do.KbID, id+ext))
	os.Remove(filepath.Join(h.dataDir, "parsed", id+".md"))

	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) toggleDocument(c *gin.Context) {
	id := c.Param("docId")

	var sample rag.ChunkDO
	currentEnabled := 0
	if err := h.db.WithContext(c.Request.Context()).
		Where("doc_id = ? AND deleted = 0", id).First(&sample).Error; err == nil {
		currentEnabled = sample.Enabled
	}
	newEnabled := 0
	if currentEnabled == 0 {
		newEnabled = 1
	}

	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).
		Where("doc_id = ? AND deleted = 0", id).
		Update("enabled", newEnabled)
	c.JSON(http.StatusOK, response.Success(gin.H{"enabled": newEnabled}))
}
