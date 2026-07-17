package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
)

func (h *Handler) listIngestionTasks(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.IngestionTask.List(c.Request.Context(), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) getIngestionTask(c *gin.Context) {
	vo, err := h.svc.IngestionTask.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) getIngestionTaskNodes(c *gin.Context) {
	nodes, err := h.svc.IngestionTask.GetNodes(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, nodes)
}
