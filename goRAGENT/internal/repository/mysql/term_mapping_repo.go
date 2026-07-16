package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// termMappingRepo TermMappingRepository 的 GORM 实现。
type termMappingRepo struct{ db *gorm.DB }

// NewTermMappingRepo 创建同义词映射 repository。
func NewTermMappingRepo(db *gorm.DB) repository.TermMappingRepository {
	return &termMappingRepo{db: db}
}

// ListEnabled 启用映射列表（对照 MappingLoader.fromDB：enabled=1 AND deleted=0，排序在内存完成）。
func (r *termMappingRepo) ListEnabled(ctx context.Context) ([]model.TermMappingDO, error) {
	var dos []model.TermMappingDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("enabled = 1").Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list enabled term mappings: %w", err)
	}
	return dos, nil
}

// List 分页查询（对照 listMappings：keyword 匹配 source_term/target_term，
// ORDER BY priority DESC, id DESC）。
func (r *termMappingRepo) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.TermMappingDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.TermMappingDO{}).Scopes(notDeleted)
	if keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("source_term LIKE ? OR target_term LIKE ?", like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count term mappings: %w", err)
	}
	var dos []model.TermMappingDO
	if err := tx.Order("priority DESC, id DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list term mappings: %w", err)
	}
	return dos, total, nil
}

func (r *termMappingRepo) FindByID(ctx context.Context, id string) (*model.TermMappingDO, error) {
	var do model.TermMappingDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find term mapping id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *termMappingRepo) Create(ctx context.Context, m *model.TermMappingDO) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create term mapping: %w", err)
	}
	return nil
}

func (r *termMappingRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.TermMappingDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update term mapping id=%s: %w", id, err)
	}
	return nil
}

func (r *termMappingRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.TermMappingDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete term mapping id=%s: %w", id, err)
	}
	return nil
}
