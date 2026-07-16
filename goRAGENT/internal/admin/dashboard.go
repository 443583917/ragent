package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/rag"
)

// StatsVO 仪表板统计
type statsVO struct {
	TotalConversations int64 `json:"totalConversations"`
	TotalMessages      int64 `json:"totalMessages"`
	TotalKb            int64 `json:"totalKb"`
	TotalDocuments     int64 `json:"totalDocuments"`
}

func (h *Handler) dashboardStatsReal(c *gin.Context) {
	var conversations, messages, kb, docs int64
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").Where("deleted = 0").Count(&conversations)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation_message").Where("deleted = 0").Count(&messages)
	h.db.WithContext(c.Request.Context()).Model(&rag.KnowledgeBaseDO{}).Where("deleted = 0").Count(&kb)
	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("deleted = 0").Count(&docs)

	c.JSON(http.StatusOK, response.Success(statsVO{
		TotalConversations: conversations, TotalMessages: messages,
		TotalKb: kb, TotalDocuments: docs,
	}))
}

func (h *Handler) dashboardOverviewReal(c *gin.Context) {
	// 24h 统计
	since := time.Now().Add(-24 * time.Hour)
	var sessions24h, messages24h, activeUsers int64
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Where("deleted = 0 AND create_time >= ?", since).Count(&sessions24h)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation_message").
		Where("deleted = 0 AND create_time >= ?", since).Count(&messages24h)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Where("deleted = 0 AND create_time >= ?", since).
		Distinct("user_id").Count(&activeUsers)

	kpi := func(v float64, d float64) gin.H { return gin.H{"value": v, "deltaPct": d} }

	// 意图热度 Top 5（按 conversation 数）
	type intentStat struct {
		Name  string
		Count int64
	}
	topIntents := []gin.H{}
	rows, _ := h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Select("COALESCE(title,'未命名') as name, COUNT(*) as count").
		Where("deleted = 0 AND create_time >= ?", since).
		Group("title").Order("count DESC").Limit(5).Rows()
	if rows != nil {
		for rows.Next() {
			var name string
			var count int64
			rows.Scan(&name, &count)
			topIntents = append(topIntents, gin.H{"name": name, "count": count})
		}
		rows.Close()
	}

	// 最近会话
	type recentConv struct {
		Title    string `json:"title"`
		UserID   string `json:"userId"`
		LastTime string `json:"lastTime"`
	}
	recent := []gin.H{}
	var convs []struct {
		Title    string
		UserID   string
		LastTime time.Time
	}
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Select("COALESCE(title,'未命名') as title, user_id, last_time").
		Where("deleted = 0").Order("last_time DESC").Limit(10).Find(&convs)
	for _, cv := range convs {
		recent = append(recent, gin.H{"title": cv.Title, "userId": cv.UserID, "lastTime": cv.LastTime.Format("2006-01-02 15:04:05")})
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"kpis":                 gin.H{"sessions24h": kpi(float64(sessions24h), 0), "messages24h": kpi(float64(messages24h), 0), "activeUsers": kpi(float64(activeUsers), 0)},
		"topIntentNodes":       topIntents,
		"recentConversations":  recent,
	}))
}

func (h *Handler) dashboardPerformanceReal(c *gin.Context) {
	var total, noDoc, errCount int64
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Count(&total)
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Where("status = 'EMPTY' AND error_message = ''").Count(&noDoc)
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Where("error_message != ''").Count(&errCount)

	var emptyRate, errorRate float64
	if total > 0 {
		emptyRate = float64(noDoc) / float64(total) * 100
		errorRate = float64(errCount) / float64(total) * 100
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"p50Ms": 0, "p95Ms": 0, "p99Ms": 0,
		"throughput":    total,
		"noDocRate":     emptyRate,
		"avgLatencyMs":  0,
		"qualityScore":  0,
		"tokenTotal":    0,
		"tokenAvg":      0,
		"errorRate":     errorRate,
		"emptyRate":     emptyRate,
	}))
}

func (h *Handler) dashboardTrendsReal(c *gin.Context) {
	// 按天统计 7 日趋势
	series := []gin.H{}
	for i := 6; i >= 0; i-- {
		dayStart := time.Now().AddDate(0, 0, -i).Truncate(24 * time.Hour)
		dayEnd := dayStart.Add(24 * time.Hour)
		var count int64
		h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
			Where("deleted = 0 AND create_time >= ? AND create_time < ?", dayStart, dayEnd).Count(&count)
		series = append(series, gin.H{"date": dayStart.Format("01-02"), "count": count})
	}

	c.JSON(http.StatusOK, response.Success(gin.H{"series": series}))
}

// RegisterRealDashboardRoutes 替换空壳路由
func (h *Handler) RegisterRealDashboardRoutes(r *gin.RouterGroup) {
	r.GET("/stats", h.dashboardStatsReal)
	r.GET("/overview", h.dashboardOverviewReal)
	r.GET("/performance", h.dashboardPerformanceReal)
	r.GET("/trends", h.dashboardTrendsReal)
}
