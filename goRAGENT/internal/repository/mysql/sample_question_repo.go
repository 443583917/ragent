package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// sampleQuestionRepo SampleQuestionRepository 的 GORM 实现。
type sampleQuestionRepo struct{ db *gorm.DB }

// NewSampleQuestionRepo 创建示例问题 repository。
func NewSampleQuestionRepo(db *gorm.DB) repository.SampleQuestionRepository {
	return &sampleQuestionRepo{db: db}
}

// List 分页查询（对照 listSampleQuestions：keyword 匹配 question/title，ORDER BY sort_order ASC）。
func (r *sampleQuestionRepo) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.SampleQuestionDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.SampleQuestionDO{}).Scopes(notDeleted)
	if keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("question LIKE ? OR title LIKE ?", like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count sample questions: %w", err)
	}
	var dos []model.SampleQuestionDO
	if err := tx.Order("sort_order ASC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list sample questions: %w", err)
	}
	return dos, total, nil
}

// ListPublic 公开示例问题（对照 getSampleQuestionsPublic：deleted=0 AND enabled=1，
// ORDER BY sort_order ASC LIMIT n）。
func (r *sampleQuestionRepo) ListPublic(ctx context.Context, limit int) ([]model.SampleQuestionDO, error) {
	var dos []model.SampleQuestionDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("enabled = 1").Order("sort_order ASC").Limit(limit).
		Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list public sample questions: %w", err)
	}
	return dos, nil
}

func (r *sampleQuestionRepo) Create(ctx context.Context, s *model.SampleQuestionDO) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("create sample question: %w", err)
	}
	return nil
}

func (r *sampleQuestionRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.SampleQuestionDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update sample question id=%s: %w", id, err)
	}
	return nil
}

func (r *sampleQuestionRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.SampleQuestionDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete sample question id=%s: %w", id, err)
	}
	return nil
}
