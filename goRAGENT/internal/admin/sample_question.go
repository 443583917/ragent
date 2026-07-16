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

type sampleQuestionVO struct {
	ID        string `json:"id"`
	Question  string `json:"question"`
	SortOrder int    `json:"sortOrder"`
	Enabled   int    `json:"enabled"`
}

type sampleQuestionCreateReq struct {
	Question  string `json:"question" binding:"required"`
	SortOrder int    `json:"sortOrder"`
}

type sampleQuestionUpdateReq struct {
	Question  *string `json:"question"`
	SortOrder *int    `json:"sortOrder"`
	Enabled   *int    `json:"enabled"`
}

func (h *Handler) listSampleQuestions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.SampleQuestionDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.SampleQuestionDO{}).Where("deleted = 0").Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("deleted = 0").Order("sort_order ASC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]sampleQuestionVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, sampleQuestionVO{ID: d.ID, Question: d.Question, SortOrder: d.SortOrder, Enabled: d.Enabled})
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getSampleQuestionsPublic(c *gin.Context) {
	var dos []rag.SampleQuestionDO
	h.db.WithContext(c.Request.Context()).
		Where("deleted = 0 AND enabled = 1").Order("sort_order ASC").Limit(10).
		Find(&dos)

	vos := make([]sampleQuestionVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, sampleQuestionVO{ID: d.ID, Question: d.Question, SortOrder: d.SortOrder})
	}
	if vos == nil {
		vos = []sampleQuestionVO{}
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) createSampleQuestion(c *gin.Context) {
	var req sampleQuestionCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "question 不能为空"))
		return
	}

	do := rag.SampleQuestionDO{
		ID: snowflake.NextID(), Question: req.Question, SortOrder: req.SortOrder, Enabled: 1,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"id": do.ID}))
}

func (h *Handler) updateSampleQuestion(c *gin.Context) {
	var req sampleQuestionUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := map[string]any{}
	if req.Question != nil {
		updates["question"] = *req.Question
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}
	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.SampleQuestionDO{}).Where("id = ?", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteSampleQuestion(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.SampleQuestionDO{}).Where("id = ?", c.Param("id")).
		Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
