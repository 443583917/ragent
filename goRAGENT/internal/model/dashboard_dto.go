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
