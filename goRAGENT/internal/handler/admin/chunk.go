package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

type chunkVO struct {
	ID              string `json:"id"`
	DocID           string `json:"docId"`
	KbID            string `json:"kbId"`
	ChunkIndex      int    `json:"chunkIndex"`
	Text            string `json:"text"`
	CharCount       int    `json:"charCount"`
	TokenCount      int    `json:"tokenCount"`
	EmbeddingStatus string `json:"embeddingStatus"`
	Enabled         int    `json:"enabled"`
}

func chunkDOtoVO(d model.ChunkDO) chunkVO {
	return chunkVO{
		ID: d.ID, DocID: d.DocID, KbID: d.KbID, ChunkIndex: d.ChunkIndex,
		Text: d.Text, CharCount: d.CharCount, TokenCount: d.TokenCount,
		EmbeddingStatus: d.EmbeddingStatus, Enabled: d.Enabled,
	}
}

func (h *Handler) listChunksByKB(c *gin.Context) {
	kbID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []model.ChunkDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&model.ChunkDO{}).
		Where("kb_id = ? AND deleted = 0", kbID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("kb_id = ? AND deleted = 0", kbID).
		Order("chunk_index ASC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]chunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, chunkDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) listChunks(c *gin.Context) {
	docID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []model.ChunkDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&model.ChunkDO{}).
		Where("doc_id = ? AND deleted = 0", docID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("doc_id = ? AND deleted = 0", docID).
		Order("chunk_index ASC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]chunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, chunkDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getChunk(c *gin.Context) {
	var do model.ChunkDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Chunk 不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(chunkDOtoVO(do)))
}

type chunkUpdateReq struct {
	Text *string `json:"text"`
}

func (h *Handler) updateChunk(c *gin.Context) {
	var req chunkUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := map[string]any{}
	if req.Text != nil {
		updates["text"] = *req.Text
		updates["char_count"] = len([]rune(*req.Text))
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.ChunkDO{}).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新 Chunk 失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) toggleChunk(c *gin.Context) {
	var do model.ChunkDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Chunk 不存在"))
		return
	}
	newEnabled := 0
	if do.Enabled == 0 {
		newEnabled = 1
	}
	h.db.WithContext(c.Request.Context()).Model(&do).Update("enabled", newEnabled)
	c.JSON(http.StatusOK, response.Success(gin.H{"enabled": newEnabled}))
}
