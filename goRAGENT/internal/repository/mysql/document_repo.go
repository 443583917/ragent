package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// documentRepo DocumentRepository 的 GORM 实现。
type documentRepo struct{ db *gorm.DB }

// NewDocumentRepo 创建文档 repository。
func NewDocumentRepo(db *gorm.DB) repository.DocumentRepository {
	return &documentRepo{db: db}
}

func (r *documentRepo) ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.DocumentDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.DocumentDO{}).Scopes(notDeleted).Where("kb_id = ?", kbID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count documents kb_id=%s: %w", kbID, err)
	}
	var dos []model.DocumentDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list documents kb_id=%s: %w", kbID, err)
	}
	return dos, total, nil
}

// Search 全局文档搜索（对照 searchDocuments：keyword 非空时 file_name LIKE）。
func (r *documentRepo) Search(ctx context.Context, keyword string, q model.PageQuery) ([]model.DocumentDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.DocumentDO{}).Scopes(notDeleted)
	if keyword != "" {
		tx = tx.Where("file_name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count documents keyword=%s: %w", keyword, err)
	}
	var dos []model.DocumentDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("search documents keyword=%s: %w", keyword, err)
	}
	return dos, total, nil
}

func (r *documentRepo) FindByID(ctx context.Context, id string) (*model.DocumentDO, error) {
	var do model.DocumentDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find document id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *documentRepo) FindByIDs(ctx context.Context, ids []string) ([]model.DocumentDO, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var dos []model.DocumentDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id IN ?", ids).Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("find documents by ids: %w", err)
	}
	return dos, nil
}

func (r *documentRepo) Create(ctx context.Context, d *model.DocumentDO) error {
	if err := r.db.WithContext(ctx).Create(d).Error; err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	return nil
}

func (r *documentRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.DocumentDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update document id=%s: %w", id, err)
	}
	return nil
}

func (r *documentRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.DocumentDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete document id=%s: %w", id, err)
	}
	return nil
}
