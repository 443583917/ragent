package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// summaryRepo SummaryRepository 的 GORM 实现。
type summaryRepo struct{ db *gorm.DB }

// NewSummaryRepo 创建会话摘要 repository。
func NewSummaryRepo(db *gorm.DB) repository.SummaryRepository {
	return &summaryRepo{db: db}
}

// Latest 最新一条摘要（对照 loadLatestSummary：deleted=0，ORDER BY id DESC LIMIT 1）。
func (r *summaryRepo) Latest(ctx context.Context, convID string) (*model.ConversationSummaryDO, error) {
	var do model.ConversationSummaryDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("conversation_id = ?", convID).
		Order("id DESC").Limit(1).
		Take(&do).Error; err != nil {
		return nil, fmt.Errorf("find latest summary conversation_id=%s: %w", convID, err)
	}
	return &do, nil
}

func (r *summaryRepo) Create(ctx context.Context, s *model.ConversationSummaryDO) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("create summary: %w", err)
	}
	return nil
}
