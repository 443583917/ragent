package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// ingestionTaskRepo IngestionTaskRepository 的 GORM 实现。
type ingestionTaskRepo struct{ db *gorm.DB }

// NewIngestionTaskRepo 创建入库任务 repository。
func NewIngestionTaskRepo(db *gorm.DB) repository.IngestionTaskRepository {
	return &ingestionTaskRepo{db: db}
}

// List 分页查询（对照 listIngestionTasks：无 deleted 过滤，ORDER BY create_time DESC）。
func (r *ingestionTaskRepo) List(ctx context.Context, q model.PageQuery) ([]model.IngestionTaskDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.IngestionTaskDO{})
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count ingestion tasks: %w", err)
	}
	var dos []model.IngestionTaskDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list ingestion tasks: %w", err)
	}
	return dos, total, nil
}

func (r *ingestionTaskRepo) FindByID(ctx context.Context, id string) (*model.IngestionTaskDO, error) {
	var do model.IngestionTaskDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find ingestion task id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *ingestionTaskRepo) Create(ctx context.Context, t *model.IngestionTaskDO) error {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("create ingestion task: %w", err)
	}
	return nil
}

func (r *ingestionTaskRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.IngestionTaskDO{}).
		Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update ingestion task id=%s: %w", id, err)
	}
	return nil
}
