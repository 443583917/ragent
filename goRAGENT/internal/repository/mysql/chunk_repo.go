package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// chunkRepo ChunkRepository 的 GORM 实现。
type chunkRepo struct{ db *gorm.DB }

// NewChunkRepo 创建分块 repository。
func NewChunkRepo(db *gorm.DB) repository.ChunkRepository {
	return &chunkRepo{db: db}
}

func (r *chunkRepo) ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.ChunkDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.ChunkDO{}).Scopes(notDeleted).Where("kb_id = ?", kbID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count chunks kb_id=%s: %w", kbID, err)
	}
	var dos []model.ChunkDO
	if err := tx.Order("chunk_index ASC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list chunks kb_id=%s: %w", kbID, err)
	}
	return dos, total, nil
}

func (r *chunkRepo) ListByDoc(ctx context.Context, docID string, q model.PageQuery) ([]model.ChunkDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.ChunkDO{}).Scopes(notDeleted).Where("doc_id = ?", docID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count chunks doc_id=%s: %w", docID, err)
	}
	var dos []model.ChunkDO
	if err := tx.Order("chunk_index ASC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list chunks doc_id=%s: %w", docID, err)
	}
	return dos, total, nil
}

func (r *chunkRepo) FindByID(ctx context.Context, id string) (*model.ChunkDO, error) {
	var do model.ChunkDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find chunk id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *chunkRepo) FindByIDs(ctx context.Context, ids []string) ([]model.ChunkDO, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var dos []model.ChunkDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id IN ?", ids).Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("find chunks by ids: %w", err)
	}
	return dos, nil
}

func (r *chunkRepo) BatchCreate(ctx context.Context, chunks []model.ChunkDO) error {
	if len(chunks) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&chunks).Error; err != nil {
		return fmt.Errorf("batch create chunks: %w", err)
	}
	return nil
}

func (r *chunkRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ChunkDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update chunk id=%s: %w", id, err)
	}
	return nil
}

// UpdateFieldsByDoc 按 doc_id 批量更新未删除分块（对照 toggleDocument）。
func (r *chunkRepo) UpdateFieldsByDoc(ctx context.Context, docID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ChunkDO{}).
		Scopes(notDeleted).Where("doc_id = ?", docID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update chunks doc_id=%s: %w", docID, err)
	}
	return nil
}

func (r *chunkRepo) SoftDeleteByDoc(ctx context.Context, docID string) error {
	if err := r.db.WithContext(ctx).Model(&model.ChunkDO{}).
		Where("doc_id = ?", docID).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete chunks doc_id=%s: %w", docID, err)
	}
	return nil
}
