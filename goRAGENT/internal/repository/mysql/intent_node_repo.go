package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// intentNodeRepo IntentNodeRepository 的 GORM 实现。
type intentNodeRepo struct{ db *gorm.DB }

// NewIntentNodeRepo 创建意图节点 repository。
func NewIntentNodeRepo(db *gorm.DB) repository.IntentNodeRepository {
	return &intentNodeRepo{db: db}
}

// ListActive 启用节点列表（对照 TreeLoader.fromDB：deleted=0 AND enabled=1，树排序在内存完成）。
func (r *intentNodeRepo) ListActive(ctx context.Context) ([]model.IntentNodeDO, error) {
	var dos []model.IntentNodeDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("enabled = 1").Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list active intent nodes: %w", err)
	}
	return dos, nil
}

// ListAll 全部未删除节点（对照 intentTrees：deleted=0，含禁用节点）。
func (r *intentNodeRepo) ListAll(ctx context.Context) ([]model.IntentNodeDO, error) {
	var dos []model.IntentNodeDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list all intent nodes: %w", err)
	}
	return dos, nil
}

func (r *intentNodeRepo) Create(ctx context.Context, n *model.IntentNodeDO) error {
	if err := r.db.WithContext(ctx).Create(n).Error; err != nil {
		return fmt.Errorf("create intent node: %w", err)
	}
	return nil
}

func (r *intentNodeRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.IntentNodeDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update intent node id=%s: %w", id, err)
	}
	return nil
}

func (r *intentNodeRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.IntentNodeDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete intent node id=%s: %w", id, err)
	}
	return nil
}

// BatchUpdateFields 批量更新（对照 batchUpdateIntent：id IN ?，无 deleted 过滤）。
func (r *intentNodeRepo) BatchUpdateFields(ctx context.Context, ids []string, updates map[string]any) error {
	if len(ids) == 0 || len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.IntentNodeDO{}).
		Where("id IN ?", ids).Updates(updates).Error; err != nil {
		return fmt.Errorf("batch update intent nodes: %w", err)
	}
	return nil
}
