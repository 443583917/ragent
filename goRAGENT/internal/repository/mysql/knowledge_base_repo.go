package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// knowledgeBaseRepo KnowledgeBaseRepository 的 GORM 实现。
type knowledgeBaseRepo struct{ db *gorm.DB }

// NewKnowledgeBaseRepo 创建知识库 repository。
func NewKnowledgeBaseRepo(db *gorm.DB) repository.KnowledgeBaseRepository {
	return &knowledgeBaseRepo{db: db}
}

func (r *knowledgeBaseRepo) List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).Scopes(notDeleted)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count knowledge bases: %w", err)
	}
	var dos []model.KnowledgeBaseDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list knowledge bases: %w", err)
	}
	return dos, total, nil
}

func (r *knowledgeBaseRepo) FindByID(ctx context.Context, id string) (*model.KnowledgeBaseDO, error) {
	var do model.KnowledgeBaseDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find knowledge base id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *knowledgeBaseRepo) Create(ctx context.Context, kb *model.KnowledgeBaseDO) error {
	if err := r.db.WithContext(ctx).Create(kb).Error; err != nil {
		return fmt.Errorf("create knowledge base: %w", err)
	}
	return nil
}

func (r *knowledgeBaseRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update knowledge base id=%s: %w", id, err)
	}
	return nil
}

func (r *knowledgeBaseRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete knowledge base id=%s: %w", id, err)
	}
	return nil
}
