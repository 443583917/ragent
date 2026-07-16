package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// dashboardRepo DashboardRepository 的 GORM 实现（读模型聚合，语义照抄原 dashboard.go）。
type dashboardRepo struct{ db *gorm.DB }

// NewDashboardRepo 创建仪表盘 repository。
func NewDashboardRepo(db *gorm.DB) repository.DashboardRepository {
	return &dashboardRepo{db: db}
}

// Stats 总量统计（对照 dashboardStatsReal）。
func (r *dashboardRepo) Stats(ctx context.Context) (*model.DashboardStats, error) {
	var s model.DashboardStats
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
		Count(&s.TotalConversations).Error; err != nil {
		return nil, fmt.Errorf("count conversations: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).Scopes(notDeleted).
		Count(&s.TotalMessages).Error; err != nil {
		return nil, fmt.Errorf("count messages: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).Scopes(notDeleted).
		Count(&s.TotalKb).Error; err != nil {
		return nil, fmt.Errorf("count knowledge bases: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.DocumentDO{}).Scopes(notDeleted).
		Count(&s.TotalDocuments).Error; err != nil {
		return nil, fmt.Errorf("count documents: %w", err)
	}
	return &s, nil
}

// Overview 概览 KPI（对照 dashboardOverviewReal，since 为窗口起点）。
func (r *dashboardRepo) Overview(ctx context.Context, since time.Time) (*model.DashboardOverview, error) {
	var o model.DashboardOverview
	if err := r.db.WithContext(ctx).Model(&model.UserDO{}).Scopes(notDeleted).
		Count(&o.TotalUsers).Error; err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
		Where("create_time >= ?", since).Distinct("user_id").
		Count(&o.ActiveUsers).Error; err != nil {
		return nil, fmt.Errorf("count active users: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
		Count(&o.TotalSessions).Error; err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
		Where("create_time >= ?", since).Count(&o.Sessions24h).Error; err != nil {
		return nil, fmt.Errorf("count window sessions: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).Scopes(notDeleted).
		Count(&o.TotalMessages).Error; err != nil {
		return nil, fmt.Errorf("count total messages: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).Scopes(notDeleted).
		Where("create_time >= ?", since).Count(&o.Messages24h).Error; err != nil {
		return nil, fmt.Errorf("count window messages: %w", err)
	}
	return &o, nil
}

// Performance 性能指标（对照 dashboardPerformanceReal：现状统计全量 trace，未按 since 过滤，
// 延迟/慢查询指标暂为 0——保持原行为，不做性能重设计）。
func (r *dashboardRepo) Performance(ctx context.Context, _ time.Time) (*model.DashboardPerformance, error) {
	var total, noDoc, errCount int64
	if err := r.db.WithContext(ctx).Model(&model.TraceRunDO{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count trace runs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.TraceRunDO{}).
		Where("status = 'EMPTY' AND error_message = ''").Count(&noDoc).Error; err != nil {
		return nil, fmt.Errorf("count no-doc trace runs: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.TraceRunDO{}).
		Where("error_message != ''").Count(&errCount).Error; err != nil {
		return nil, fmt.Errorf("count error trace runs: %w", err)
	}

	p := model.DashboardPerformance{SuccessRate: 100.0}
	if total > 0 {
		p.ErrorRate = round2(float64(errCount) / float64(total) * 100)
		p.NoDocRate = round2(float64(noDoc) / float64(total) * 100)
		p.SuccessRate = round2(100.0 - p.ErrorRate - p.NoDocRate)
		if p.SuccessRate < 0 {
			p.SuccessRate = 0
		}
	}
	return &p, nil
}

// TrendCounts 按时间桶统计指标数量（对照 dashboardTrendsReal 的逐桶 Count 现状；
// 未知指标与 avgLatency/quality 返回全 0，与原实现一致）。
func (r *dashboardRepo) TrendCounts(ctx context.Context, metric string, buckets []model.TimeBucket) ([]int64, error) {
	counts := make([]int64, len(buckets))
	for i, b := range buckets {
		var count int64
		var err error
		switch metric {
		case model.TrendMetricSessions:
			err = r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
				Where("create_time >= ? AND create_time < ?", b.Start, b.End).
				Count(&count).Error
		case model.TrendMetricMessages:
			err = r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).Scopes(notDeleted).
				Where("create_time >= ? AND create_time < ?", b.Start, b.End).
				Count(&count).Error
		case model.TrendMetricActiveUsers:
			err = r.db.WithContext(ctx).Model(&model.ConversationDO{}).Scopes(notDeleted).
				Where("create_time >= ? AND create_time < ?", b.Start, b.End).
				Distinct("user_id").Count(&count).Error
		case model.TrendMetricTraceRuns:
			err = r.db.WithContext(ctx).Model(&model.TraceRunDO{}).
				Where("create_time >= ? AND create_time < ?", b.Start, b.End).
				Count(&count).Error
		case model.TrendMetricTraceNoDoc:
			err = r.db.WithContext(ctx).Model(&model.TraceRunDO{}).
				Where("status = 'EMPTY' AND error_message = '' AND create_time >= ? AND create_time < ?", b.Start, b.End).
				Count(&count).Error
		default:
			count = 0
		}
		if err != nil {
			return nil, fmt.Errorf("trend count metric=%s bucket=%d: %w", metric, i, err)
		}
		counts[i] = count
	}
	return counts, nil
}

// round2 保留两位小数（照抄原 dashboard.go round2）。
func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
