package model

import "time"

// DashboardStats 仪表盘总量统计（对照 dashboardStatsReal 响应字段）。
type DashboardStats struct {
	TotalConversations int64
	TotalMessages      int64
	TotalKb            int64
	TotalDocuments     int64
}

// DashboardOverview 仪表盘概览 KPI（对照 dashboardOverviewReal 响应字段）。
type DashboardOverview struct {
	TotalUsers    int64
	ActiveUsers   int64
	TotalSessions int64
	Sessions24h   int64 // 窗口内新建会话数（响应字段 sessions24h，窗口由 since 决定）
	TotalMessages int64
	Messages24h   int64 // 窗口内新增消息数（响应字段 messages24h）
}

// DashboardPerformance 仪表盘性能指标（对照 dashboardPerformanceReal 响应字段）。
type DashboardPerformance struct {
	AvgLatencyMs float64
	P95LatencyMs float64
	SuccessRate  float64
	ErrorRate    float64
	NoDocRate    float64
	SlowRate     float64
}

// TimeBucket 趋势统计时间桶，区间为 [Start, End)。
type TimeBucket struct {
	Start time.Time
	End   time.Time
}

// ========== Dashboard 响应 VO ==========

// KpiVO 概览 KPI 值（带同比变化量）。
type KpiVO struct {
	Value    float64 `json:"value"`
	Delta    float64 `json:"delta,omitempty"`
	DeltaPct float64 `json:"deltaPct,omitempty"`
}

// OverviewResp 概览响应。
type OverviewResp struct {
	Window        string           `json:"window"`
	CompareWindow string           `json:"compareWindow"`
	UpdatedAt     int64            `json:"updatedAt"`
	Kpis          map[string]KpiVO `json:"kpis"`
}

// PerformanceResp 性能指标响应。
type PerformanceResp struct {
	Window       string  `json:"window"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	P95LatencyMs float64 `json:"p95LatencyMs"`
	SuccessRate  float64 `json:"successRate"`
	ErrorRate    float64 `json:"errorRate"`
	NoDocRate    float64 `json:"noDocRate"`
	SlowRate     float64 `json:"slowRate"`
}

// TrendPoint 趋势数据点。
type TrendPoint struct {
	Ts    int64 `json:"ts"`
	Value int64 `json:"value"`
}

// TrendSeries 趋势序列（含名称和数据点列表）。
type TrendSeries struct {
	Name string       `json:"name"`
	Data []TrendPoint `json:"data"`
}

// TrendsResp 趋势响应。
type TrendsResp struct {
	Metric      string        `json:"metric"`
	Window      string        `json:"window"`
	Granularity string        `json:"granularity"`
	Series      []TrendSeries `json:"series"`
}
