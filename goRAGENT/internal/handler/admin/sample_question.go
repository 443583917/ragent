package admin

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/pkg/snowflake"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

// sampleQuestionItemVO 匹配前端 SampleQuestion 类型
type sampleQuestionItemVO struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Question    string `json:"question"`
	CreateTime  string `json:"createTime,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
}

// pageResultVO 匹配前端 PageResult<T> 类型
type pageResultVO struct {
	Records []sampleQuestionItemVO `json:"records"`
	Total   int64                  `json:"total"`
	Size    int                    `json:"size"`
	Current int                    `json:"current"`
	Pages   int                    `json:"pages"`
}

type sampleQuestionPayload struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Question    *string `json:"question"`
}

func sqDOtoItem(d model.SampleQuestionDO) sampleQuestionItemVO {
	return sampleQuestionItemVO{
		ID: d.ID, Title: d.Title, Description: d.Description,
		Question: d.Question,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
		UpdateTime: d.UpdateTime.Format("2006-01-02 15:04:05"),
	}
}

func (h *Handler) listSampleQuestions(c *gin.Context) {
	current, _ := strconv.Atoi(c.DefaultQuery("current", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if current < 1 {
		current = 1
	}
	if size < 1 {
		size = 10
	}
	keyword := c.Query("keyword")

	var dos []model.SampleQuestionDO
	var total int64
	query := h.db.WithContext(c.Request.Context()).Model(&model.SampleQuestionDO{}).Where("deleted = 0")
	if keyword != "" {
		query = query.Where("question LIKE ? OR title LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	query.Count(&total)
	if err := query.Order("sort_order ASC").Offset((current - 1) * size).Limit(size).Find(&dos).Error; err != nil {
		zap.L().Error("查询示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]sampleQuestionItemVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, sqDOtoItem(d))
	}
	if vos == nil {
		vos = []sampleQuestionItemVO{}
	}
	pages := int(math.Ceil(float64(total) / float64(size)))

	c.JSON(http.StatusOK, response.Success(pageResultVO{
		Records: vos, Total: total, Size: size, Current: current, Pages: pages,
	}))
}

func (h *Handler) getSampleQuestionsPublic(c *gin.Context) {
	var dos []model.SampleQuestionDO
	h.db.WithContext(c.Request.Context()).
		Where("deleted = 0 AND enabled = 1").Order("sort_order ASC").Limit(10).
		Find(&dos)

	vos := make([]sampleQuestionItemVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, sqDOtoItem(d))
	}
	if vos == nil {
		vos = []sampleQuestionItemVO{}
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) createSampleQuestion(c *gin.Context) {
	var req sampleQuestionPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	if req.Question == nil || *req.Question == "" {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "question 不能为空"))
		return
	}

	title := ""
	if req.Title != nil {
		title = *req.Title
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}

	do := model.SampleQuestionDO{
		ID: snowflake.NextID(), Title: title, Description: desc,
		Question: *req.Question, SortOrder: 0, Enabled: 1,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(do.ID))
}

func (h *Handler) updateSampleQuestion(c *gin.Context) {
	var req sampleQuestionPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := map[string]any{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Question != nil {
		updates["question"] = *req.Question
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.SampleQuestionDO{}).Where("id = ?", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteSampleQuestion(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.SampleQuestionDO{}).Where("id = ?", c.Param("id")).
		Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除示例问题失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
