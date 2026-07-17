package admin

import (
	"strings"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
)

func (h *Handler) listMappings(c *gin.Context) {
	pq := httpx.PageFromCurrentSize(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	vos, total, err := h.svc.Mapping.List(c.Request.Context(), pq, keyword)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, model.NewPageResult(vos, total, pq))
}

func (h *Handler) getMapping(c *gin.Context) {
	vo, err := h.svc.Mapping.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) createMapping(c *gin.Context) {
	var req model.MappingCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "sourceTerm/targetTerm 不能为空")
		return
	}
	id, err := h.svc.Mapping.Create(c.Request.Context(), req, middleware.GetUserID(c.Request.Context()))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, id)
}

func (h *Handler) updateMapping(c *gin.Context) {
	var req model.MappingUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Mapping.Update(c.Request.Context(), c.Param("id"), req, middleware.GetUserID(c.Request.Context())); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) deleteMapping(c *gin.Context) {
	if err := h.svc.Mapping.Delete(c.Request.Context(), c.Param("id"), middleware.GetUserID(c.Request.Context())); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}
