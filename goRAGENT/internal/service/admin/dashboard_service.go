package admin

import (
	"context"
	"time"

	"go.uber.org/zap"
	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
)

// DashboardService 仪表盘读模型服务接口。
type DashboardService interface {
	Stats(ctx context.Context) (*model.DashboardStats, error)
	Overview(ctx context.Context, window string) (*model.OverviewResp, error)
	Performance(ctx context.Context, window string) (*model.PerformanceResp, error)
	Trends(ctx context.Context, metric, window, granularity string) (*model.TrendsResp, error)
}

type dashboardService struct {
	repo repository.DashboardRepository
}

// NewDashboardService 创建仪表盘服务。
func NewDashboardService(repo repository.DashboardRepository) DashboardService {
	return &dashboardService{repo: repo}
}

func (s *dashboardService) Stats(ctx context.Context) (*model.DashboardStats, error) {
	stats, err := s.repo.Stats(ctx)
	if err != nil {
		zap.L().Error("查询仪表盘统计失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询失败")
	}
	return stats, nil
}

func (s *dashboardService) Overview(ctx context.Context, window string) (*model.OverviewResp, error) {
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

	ov, err := s.repo.Overview(ctx, since)
	if err != nil {
		zap.L().Error("查询仪表盘概览失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询失败")
	}

	kpi := func(v float64) model.KpiVO { return model.KpiVO{Value: v} }
	return &model.OverviewResp{
		Window:        window,
		CompareWindow: "previous_" + window,
		UpdatedAt:     now.UnixMilli(),
		Kpis: map[string]model.KpiVO{
			"totalUsers":    kpi(float64(ov.TotalUsers)),
			"activeUsers":   kpi(float64(ov.ActiveUsers)),
			"totalSessions": kpi(float64(ov.TotalSessions)),
			"sessions24h":   kpi(float64(ov.Sessions24h)),
			"totalMessages": kpi(float64(ov.TotalMessages)),
			"messages24h":   kpi(float64(ov.Messages24h)),
		},
	}, nil
}

func (s *dashboardService) Performance(ctx context.Context, window string) (*model.PerformanceResp, error) {
	var since time.Time
	switch window {
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-24 * time.Hour)
	}

	pf, err := s.repo.Performance(ctx, since)
	if err != nil {
		zap.L().Error("查询仪表盘性能失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询失败")
	}

	return &model.PerformanceResp{
		Window:       window,
		AvgLatencyMs: pf.AvgLatencyMs,
		P95LatencyMs: pf.P95LatencyMs,
		SuccessRate:  pf.SuccessRate,
		ErrorRate:    pf.ErrorRate,
		NoDocRate:    pf.NoDocRate,
		SlowRate:     pf.SlowRate,
	}, nil
}

// metricDisplayNames 指标显示名称映射（与现有 handler/metricNames 一致）。
var metricDisplayNames = map[string]string{
	"sessions":    "Sessions",
	"messages":    "Messages",
	"activeUsers": "Active Users",
	"avgLatency":  "Avg Latency",
	"quality":     "Quality",
}

func (s *dashboardService) Trends(ctx context.Context, metric, window, granularity string) (*model.TrendsResp, error) {
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
	buckets := make([]model.TimeBucket, 0, days)
	bucketStarts := make([]int64, 0, days)

	for i := days - 1; i >= 0; i-- {
		var start, end time.Time
		if granularity == "hour" {
			start = now.Add(-time.Duration(i+1) * step).Truncate(step)
			end = start.Add(step)
		} else {
			start = now.AddDate(0, 0, -i).Truncate(step)
			end = start.AddDate(0, 0, 1)
		}
		buckets = append(buckets, model.TimeBucket{Start: start, End: end})
		bucketStarts = append(bucketStarts, start.UnixMilli())
	}

	var series []model.TrendSeries

	if metric == "quality" {
		// 错误率序列
		errCounts, err := s.repo.TrendCounts(ctx, model.TrendMetricTraceRuns, buckets)
		if err != nil {
			zap.L().Error("查询趋势(错误率)失败", zap.Error(err))
			return nil, errs.WrapServer(err, "查询失败")
		}
		errPoints := make([]model.TrendPoint, len(errCounts))
		for i, c := range errCounts {
			errPoints[i] = model.TrendPoint{Ts: bucketStarts[i], Value: c}
		}

		// 无知识率序列
		noDocCounts, err := s.repo.TrendCounts(ctx, model.TrendMetricTraceNoDoc, buckets)
		if err != nil {
			zap.L().Error("查询趋势(无知识率)失败", zap.Error(err))
			return nil, errs.WrapServer(err, "查询失败")
		}
		noDocPoints := make([]model.TrendPoint, len(noDocCounts))
		for i, c := range noDocCounts {
			noDocPoints[i] = model.TrendPoint{Ts: bucketStarts[i], Value: c}
		}

		series = []model.TrendSeries{
			{Name: "错误率", Data: errPoints},
			{Name: "无知识率", Data: noDocPoints},
		}
	} else {
		// 标准指标：sessions / messages / activeUsers / avgLatency
		counts, err := s.repo.TrendCounts(ctx, metric, buckets)
		if err != nil {
			zap.L().Error("查询趋势失败", zap.Error(err))
			return nil, errs.WrapServer(err, "查询失败")
		}
		points := make([]model.TrendPoint, len(counts))
		for i, c := range counts {
			points[i] = model.TrendPoint{Ts: bucketStarts[i], Value: c}
		}

		name := metricDisplayNames[metric]
		if name == "" {
			name = metric
		}
		series = []model.TrendSeries{{Name: name, Data: points}}
	}

	return &model.TrendsResp{
		Metric: metric, Window: window, Granularity: granularity,
		Series: series,
	}, nil
}
