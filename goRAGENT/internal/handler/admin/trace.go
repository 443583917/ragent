package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listTraceRunsReal(c *gin.Context) {
	pq := httpx.PageFromCurrentSize(c)
	traceID := c.Query("traceId")
	convID := c.Query("conversationId")
	vos, total, err := h.svc.Trace.ListRuns(c.Request.Context(), pq, traceID, convID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	if vos == nil {
		vos = []model.RagTraceRunVO{}
	}
	httpx.OK(c, model.NewPageResult(vos, total, pq))
}

func (h *Handler) getTraceDetailReal(c *gin.Context) {
	runID := c.Param("traceId")
	if runID == "" {
		runID = c.Param("runId")
	}
	if runID == "" {
		httpx.BadRequest(c, "runId 不能为空")
		return
	}
	detail, err := h.svc.Trace.GetDetail(c.Request.Context(), runID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *Handler) getTraceNodesReal(c *gin.Context) {
	runID := c.Param("traceId")
	nodes, err := h.svc.Trace.GetNodes(c.Request.Context(), runID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	if nodes == nil {
		nodes = []model.RagTraceNodeVO{}
	}
	httpx.OK(c, nodes)
}

// GetTraceNodes 公开方法（供 router 调用）。
func (h *Handler) GetTraceNodes(c *gin.Context) { h.getTraceNodesReal(c) }
