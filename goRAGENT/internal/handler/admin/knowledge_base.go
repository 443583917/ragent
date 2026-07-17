package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listKnowledgeBases(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.KnowledgeBase.List(c.Request.Context(), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) createKnowledgeBase(c *gin.Context) {
	var req model.KnowledgeBaseCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name 不能为空")
		return
	}
	vo, err := h.svc.KnowledgeBase.Create(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) getKnowledgeBase(c *gin.Context) {
	vo, err := h.svc.KnowledgeBase.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) updateKnowledgeBase(c *gin.Context) {
	var req model.KnowledgeBaseUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.KnowledgeBase.Update(c.Request.Context(), c.Param("id"), req); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) deleteKnowledgeBase(c *gin.Context) {
	if err := h.svc.KnowledgeBase.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}
