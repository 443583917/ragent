package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/rag"
)

type kpiVO struct {
	Value    float64 `json:"value"`
	Delta    float64 `json:"delta,omitempty"`
	DeltaPct float64 `json:"deltaPct,omitempty"`
}

type overviewResp struct {
	Window        string           `json:"window"`
	CompareWindow string           `json:"compareWindow"`
	UpdatedAt     int64            `json:"updatedAt"`
	Kpis          map[string]kpiVO `json:"kpis"`
}

type performanceResp struct {
	Window        string  `json:"window"`
	AvgLatencyMs  float64 `json:"avgLatencyMs"`
	P95LatencyMs  float64 `json:"p95LatencyMs"`
	SuccessRate   float64 `json:"successRate"`
	ErrorRate     float64 `json:"errorRate"`
	NoDocRate     float64 `json:"noDocRate"`
	SlowRate      float64 `json:"slowRate"`
}

type trendPoint struct {
	Ts    int64 `json:"ts"`
	Value int64 `json:"value"`
}

type trendSeries struct {
	Name string       `json:"name"`
	Data []trendPoint `json:"data"`
}

type trendsResp struct {
	Metric      string        `json:"metric"`
	Window      string        `json:"window"`
	Granularity string        `json:"granularity"`
	Series      []trendSeries `json:"series"`
}

var metricNames = map[string]string{
	"sessions":    "Sessions",
	"messages":    "Messages",
	"activeUsers": "Active Users",
	"avgLatency":  "Avg Latency",
	"quality":     "Quality",
}

func (h *Handler) dashboardStatsReal(c *gin.Context) {
	var conversations, messages, kb, docs int64
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").Where("deleted = 0").Count(&conversations)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation_message").Where("deleted = 0").Count(&messages)
	h.db.WithContext(c.Request.Context()).Model(&rag.KnowledgeBaseDO{}).Where("deleted = 0").Count(&kb)
	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("deleted = 0").Count(&docs)

	c.JSON(http.StatusOK, response.Success(map[string]int64{
		"totalConversations": conversations, "totalMessages": messages,
		"totalKb": kb, "totalDocuments": docs,
	}))
}

func (h *Handler) dashboardOverviewReal(c *gin.Context) {
	window := c.DefaultQuery("window", "24h")
	now := time.Now()
	var since time.Time
	switch window {
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
	default:
		since = now.Add(-24 * time.Hour)
	}

	var totalUsers, activeUsers, totalSessions, sessions24h, totalMessages, messages24h int64
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_user").Where("deleted = 0").Count(&totalUsers)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Where("deleted = 0 AND create_time >= ?", since).Distinct("user_id").Count(&activeUsers)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").Where("deleted = 0").Count(&totalSessions)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
		Where("deleted = 0 AND create_time >= ?", since).Count(&sessions24h)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation_message").Where("deleted = 0").Count(&totalMessages)
	h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation_message").
		Where("deleted = 0 AND create_time >= ?", since).Count(&messages24h)

	kpi := func(v float64) kpiVO { return kpiVO{Value: v} }
	c.JSON(http.StatusOK, response.Success(overviewResp{
		Window: window, CompareWindow: "previous_" + window, UpdatedAt: now.UnixMilli(),
		Kpis: map[string]kpiVO{
			"totalUsers":    kpi(float64(totalUsers)),
			"activeUsers":   kpi(float64(activeUsers)),
			"totalSessions": kpi(float64(totalSessions)),
			"sessions24h":   kpi(float64(sessions24h)),
			"totalMessages": kpi(float64(totalMessages)),
			"messages24h":   kpi(float64(messages24h)),
		},
	}))
}

func (h *Handler) dashboardPerformanceReal(c *gin.Context) {
	window := c.DefaultQuery("window", "24h")
	var total, noDoc, errCount int64
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Count(&total)
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Where("status = 'EMPTY' AND error_message = ''").Count(&noDoc)
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Where("error_message != ''").Count(&errCount)

	successRate, errorRate, noDocRate, slowRate := 100.0, 0.0, 0.0, 0.0
	if total > 0 {
		errorRate = round2(float64(errCount) / float64(total) * 100)
		noDocRate = round2(float64(noDoc) / float64(total) * 100)
		successRate = round2(100.0 - errorRate - noDocRate)
		if successRate < 0 {
			successRate = 0
		}
	}

	c.JSON(http.StatusOK, response.Success(performanceResp{
		Window:        window,
		AvgLatencyMs:  0,
		P95LatencyMs:  0,
		SuccessRate:   successRate,
		ErrorRate:     errorRate,
		NoDocRate:     noDocRate,
		SlowRate:      slowRate,
	}))
}

func (h *Handler) dashboardTrendsReal(c *gin.Context) {
	metric := c.DefaultQuery("metric", "sessions")
	window := c.DefaultQuery("window", "7d")
	granularity := c.DefaultQuery("granularity", "day")

	var days int
	switch window {
	case "24h":
		days = 1
	case "30d":
		days = 30
	default:
		days = 7
	}

	var step time.Duration
	if granularity == "hour" {
		step = time.Hour
	} else {
		step = 24 * time.Hour
	}

	now := time.Now()
	table := "t_conversation"
	if metric == "messages" {
		table = "t_conversation_message"
	}

	var points []trendPoint
	for i := days - 1; i >= 0; i-- {
		var start, end time.Time
		if granularity == "hour" {
			start = now.Add(-time.Duration(i+1) * step).Truncate(step)
			end = start.Add(step)
		} else {
			start = now.AddDate(0, 0, -i).Truncate(step)
			end = start.AddDate(0, 0, 1)
		}
		var count int64
		if metric == "activeUsers" {
			h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table("t_conversation").
				Where("deleted = 0 AND create_time >= ? AND create_time < ?", start, end).
				Distinct("user_id").Count(&count)
		} else if metric == "avgLatency" || metric == "quality" {
			count = 0
		} else {
			h.db.WithContext(c.Request.Context()).Model(&struct{}{}).Table(table).
				Where("deleted = 0 AND create_time >= ? AND create_time < ?", start, end).Count(&count)
		}
		points = append(points, trendPoint{Ts: start.UnixMilli(), Value: count})
	}

	name := metricNames[metric]
	if name == "" {
		name = metric
	}
	series := []trendSeries{{Name: name, Data: points}}

	// quality metric returns 2 series (error rate + no-doc rate) matching Java
	if metric == "quality" {
		var errPoints, noDocPoints []trendPoint
		for i := days - 1; i >= 0; i-- {
			var start, end time.Time
			if granularity == "hour" {
				start = now.Add(-time.Duration(i+1) * step).Truncate(step)
				end = start.Add(step)
			} else {
				start = now.AddDate(0, 0, -i).Truncate(step)
				end = start.AddDate(0, 0, 1)
			}
			var runs int64
			h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).
				Where("create_time >= ? AND create_time < ?", start, end).Count(&runs)
			errPoints = append(errPoints, trendPoint{Ts: start.UnixMilli(), Value: runs})
		}
		for i := days - 1; i >= 0; i-- {
			var start, end time.Time
			if granularity == "hour" {
				start = now.Add(-time.Duration(i+1) * step).Truncate(step)
				end = start.Add(step)
			} else {
				start = now.AddDate(0, 0, -i).Truncate(step)
				end = start.AddDate(0, 0, 1)
			}
			var noDoc int64
			h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).
				Where("status = 'EMPTY' AND error_message = '' AND create_time >= ? AND create_time < ?", start, end).Count(&noDoc)
			noDocPoints = append(noDocPoints, trendPoint{Ts: start.UnixMilli(), Value: noDoc})
		}
		series = []trendSeries{
			{Name: "错误率", Data: errPoints},
			{Name: "无知识率", Data: noDocPoints},
		}
	}

	c.JSON(http.StatusOK, response.Success(trendsResp{
		Metric: metric, Window: window, Granularity: granularity,
		Series: series,
	}))
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// RegisterRealDashboardRoutes 替换空壳路由
func (h *Handler) RegisterRealDashboardRoutes(r *gin.RouterGroup) {
	r.GET("/stats", h.dashboardStatsReal)
	r.GET("/overview", h.dashboardOverviewReal)
	r.GET("/performance", h.dashboardPerformanceReal)
	r.GET("/trends", h.dashboardTrendsReal)
}
