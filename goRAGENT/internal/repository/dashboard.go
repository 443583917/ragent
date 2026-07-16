package repository

import (
	"context"
	"time"

	"goRAGENT/internal/model"
)

// DashboardRepository 仪表盘读模型聚合（语义以现有 dashboard.go 为准）。
type DashboardRepository interface {
	Stats(ctx context.Context) (*model.DashboardStats, error)
	Overview(ctx context.Context, since time.Time) (*model.DashboardOverview, error)
	Performance(ctx context.Context, since time.Time) (*model.DashboardPerformance, error)
	// TrendCounts 按时间桶统计指定指标数量；metric 取值见 model/consts.go TrendMetric* 常量
	TrendCounts(ctx context.Context, metric string, buckets []model.TimeBucket) ([]int64, error)
}
