package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listChunksByKB(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.Chunk.ListByKB(c.Request.Context(), c.Param("id"), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) listChunks(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.Chunk.ListByDoc(c.Request.Context(), c.Param("id"), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) getChunk(c *gin.Context) {
	vo, err := h.svc.Chunk.Get(c.Request.Context(), c.Param("chunkId"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) updateChunk(c *gin.Context) {
	var req model.ChunkUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Chunk.Update(c.Request.Context(), c.Param("chunkId"), req); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) toggleChunk(c *gin.Context) {
	enabled, err := h.svc.Chunk.Toggle(c.Request.Context(), c.Param("chunkId"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"enabled": enabled})
}
