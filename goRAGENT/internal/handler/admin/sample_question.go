package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listSampleQuestions(c *gin.Context) {
	pq := httpx.PageFromCurrentSize(c)
	vos, total, err := h.svc.SampleQuestion.List(c.Request.Context(), pq, c.Query("keyword"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	if vos == nil {
		vos = []model.SampleQuestionItemVO{}
	}
	httpx.OK(c, model.NewPageResult(vos, total, pq))
}

func (h *Handler) getSampleQuestionsPublic(c *gin.Context) {
	vos, err := h.svc.SampleQuestion.ListPublic(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}
	if vos == nil {
		vos = []model.SampleQuestionItemVO{}
	}
	httpx.OK(c, vos)
}

func (h *Handler) createSampleQuestion(c *gin.Context) {
	var req model.SampleQuestionPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if req.Question == nil || *req.Question == "" {
		httpx.BadRequest(c, "question 不能为空")
		return
	}
	id, err := h.svc.SampleQuestion.Create(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, id)
}

func (h *Handler) updateSampleQuestion(c *gin.Context) {
	var req model.SampleQuestionPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.SampleQuestion.Update(c.Request.Context(), c.Param("id"), req); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) deleteSampleQuestion(c *gin.Context) {
	if err := h.svc.SampleQuestion.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}
