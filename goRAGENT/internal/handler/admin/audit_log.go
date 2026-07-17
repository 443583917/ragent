package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
)

func (h *Handler) listBizChangeLogs(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.Audit.List(c.Request.Context(), pq, c.Query("entityType"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) getBizChangeLog(c *gin.Context) {
	vo, err := h.svc.Audit.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}
