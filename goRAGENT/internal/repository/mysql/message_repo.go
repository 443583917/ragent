package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// messageRepo MessageRepository 的 GORM 实现。
type messageRepo struct{ db *gorm.DB }

// NewMessageRepo 创建会话消息 repository。
func NewMessageRepo(db *gorm.DB) repository.MessageRepository {
	return &messageRepo{db: db}
}

// ListByConversation 会话全部消息（对照 ListMessages：deleted=0，ORDER BY id ASC）。
func (r *messageRepo) ListByConversation(ctx context.Context, convID string) ([]model.ConversationMessageDO, error) {
	var dos []model.ConversationMessageDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("conversation_id = ?", convID).
		Order("id ASC").
		Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list messages conversation_id=%s: %w", convID, err)
	}
	return dos, nil
}

// ListRecent 最近 limit 条消息，按 id DESC 返回（对照 loadHistory，调用方自行反转为正序）。
func (r *messageRepo) ListRecent(ctx context.Context, convID string, limit int) ([]model.ConversationMessageDO, error) {
	var dos []model.ConversationMessageDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("conversation_id = ?", convID).
		Order("id DESC").Limit(limit).
		Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list recent messages conversation_id=%s: %w", convID, err)
	}
	return dos, nil
}

// ListRange 区间消息 afterID < id < beforeID（对照 maybeCompress 待摘要查询：
// 仅 user/assistant 角色，ORDER BY id ASC）。
func (r *messageRepo) ListRange(ctx context.Context, convID string, afterID, beforeID int64) ([]model.ConversationMessageDO, error) {
	var dos []model.ConversationMessageDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("conversation_id = ? AND role IN ? AND id > ? AND id < ?",
			convID, []string{model.MsgRoleUser, model.MsgRoleAssistant}, afterID, beforeID).
		Order("id ASC").
		Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list range messages conversation_id=%s: %w", convID, err)
	}
	return dos, nil
}

// CountUserMessages 会话内 user 角色消息数（对照 maybeCompress 触发轮数判断）。
func (r *messageRepo) CountUserMessages(ctx context.Context, convID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).Scopes(notDeleted).
		Where("conversation_id = ? AND role = ?", convID, model.MsgRoleUser).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count user messages conversation_id=%s: %w", convID, err)
	}
	return count, nil
}

func (r *messageRepo) Create(ctx context.Context, m *model.ConversationMessageDO) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

// UpdateFields 按消息主键更新（对照 updateVote，id 为消息 PK 字符串）。
func (r *messageRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).
		Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update message id=%s: %w", id, err)
	}
	return nil
}
