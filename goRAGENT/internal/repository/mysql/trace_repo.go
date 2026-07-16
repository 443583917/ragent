package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// traceRepo TraceRepository 的 GORM 实现（trace 表无 deleted 列，不做软删过滤）。
type traceRepo struct{ db *gorm.DB }

// NewTraceRepo 创建 Trace repository。
func NewTraceRepo(db *gorm.DB) repository.TraceRepository {
	return &traceRepo{db: db}
}

func (r *traceRepo) CreateRun(ctx context.Context, run *model.TraceRunDO) error {
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create trace run: %w", err)
	}
	return nil
}

// UpdateRunFieldsByTaskID 按 run_id 更新（对照 chat handler：taskID 即 run_id）。
func (r *traceRepo) UpdateRunFieldsByTaskID(ctx context.Context, taskID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.TraceRunDO{}).
		Where("run_id = ?", taskID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update trace run task_id=%s: %w", taskID, err)
	}
	return nil
}

// ListRuns 分页查询（对照 listTraceRunsReal：过滤条件精确匹配，ORDER BY create_time DESC）。
func (r *traceRepo) ListRuns(ctx context.Context, q model.PageQuery, f model.TraceRunFilter) ([]model.TraceRunDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.TraceRunDO{})
	if f.TraceID != "" {
		tx = tx.Where("run_id = ?", f.TraceID)
	}
	if f.ConversationID != "" {
		tx = tx.Where("conversation_id = ?", f.ConversationID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count trace runs: %w", err)
	}
	var dos []model.TraceRunDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list trace runs: %w", err)
	}
	return dos, total, nil
}

func (r *traceRepo) FindRun(ctx context.Context, runID string) (*model.TraceRunDO, error) {
	var do model.TraceRunDO
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find trace run run_id=%s: %w", runID, err)
	}
	return &do, nil
}

// ListNodes 运行节点列表（对照 getTraceNodesReal：ORDER BY id ASC）。
func (r *traceRepo) ListNodes(ctx context.Context, runID string) ([]model.TraceNodeDO, error) {
	var dos []model.TraceNodeDO
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).
		Order("id ASC").Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list trace nodes run_id=%s: %w", runID, err)
	}
	return dos, nil
}
