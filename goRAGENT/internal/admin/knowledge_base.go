package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type knowledgeBaseVO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	EmbeddingModel string `json:"embeddingModel,omitempty"`
	CollectionName string `json:"collectionName,omitempty"`
	Dimension      int    `json:"dimension"`
	CreateTime     string `json:"createTime"`
}

type knowledgeBaseCreateReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type knowledgeBaseUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func kbDOtoVO(d rag.KnowledgeBaseDO) knowledgeBaseVO {
	return knowledgeBaseVO{
		ID: d.ID, Name: d.Name, Description: d.Description,
		EmbeddingModel: d.EmbeddingModel, CollectionName: d.CollectionName,
		Dimension: d.Dimension, CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}

func (h *Handler) listKnowledgeBases(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.KnowledgeBaseDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.KnowledgeBaseDO{}).Where("deleted = 0").Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("deleted = 0").Order("create_time DESC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询知识库列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]knowledgeBaseVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, kbDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) createKnowledgeBase(c *gin.Context) {
	var req knowledgeBaseCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "name 不能为空"))
		return
	}

	id := snowflake.NextID()
	collectionName := "kb_" + id

	if h.milvus != nil {
		if err := h.milvus.CreateCollection(c.Request.Context(), collectionName, 1536); err != nil {
			zap.L().Error("创建 Milvus Collection 失败", zap.Error(err))
			c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建向量集合失败"))
			return
		}
	}

	do := rag.KnowledgeBaseDO{
		ID: id, Name: req.Name, Description: req.Description,
		CollectionName: collectionName, Dimension: 1536,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(kbDOtoVO(do)))
}

func (h *Handler) getKnowledgeBase(c *gin.Context) {
	var do rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(kbDOtoVO(do)))
}

func (h *Handler) updateKnowledgeBase(c *gin.Context) {
	var req knowledgeBaseUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}

	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.KnowledgeBaseDO{}).
		Where("id = ? AND deleted = 0", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteKnowledgeBase(c *gin.Context) {
	id := c.Param("id")

	var do rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", id).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}

	if h.milvus != nil && do.CollectionName != "" {
		h.milvus.DropCollection(c.Request.Context(), do.CollectionName)
	}

	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.KnowledgeBaseDO{}).Where("id = ?", id).
		Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
