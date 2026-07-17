package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
)

func (h *Handler) dashboardStatsReal(c *gin.Context) {
	stats, err := h.svc.Dashboard.Stats(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"totalConversations": stats.TotalConversations,
		"totalMessages":      stats.TotalMessages,
		"totalKb":            stats.TotalKb,
		"totalDocuments":     stats.TotalDocuments,
	})
}

func (h *Handler) dashboardOverviewReal(c *gin.Context) {
	window := c.DefaultQuery("window", "24h")
	resp, err := h.svc.Dashboard.Overview(c.Request.Context(), window)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) dashboardPerformanceReal(c *gin.Context) {
	window := c.DefaultQuery("window", "24h")
	resp, err := h.svc.Dashboard.Performance(c.Request.Context(), window)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) dashboardTrendsReal(c *gin.Context) {
	metric := c.DefaultQuery("metric", "sessions")
	window := c.DefaultQuery("window", "7d")
	granularity := c.DefaultQuery("granularity", "day")
	resp, err := h.svc.Dashboard.Trends(c.Request.Context(), metric, window, granularity)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, resp)
}
