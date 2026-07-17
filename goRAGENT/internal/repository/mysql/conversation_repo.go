package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// conversationRepo ConversationRepository 的 GORM 实现（id 均指业务字段 conversation_id）。
type conversationRepo struct{ db *gorm.DB }

// NewConversationRepo 创建会话 repository。
func NewConversationRepo(db *gorm.DB) repository.ConversationRepository {
	return &conversationRepo{db: db}
}

// ListByUser 用户会话列表（对照 ListSessions：deleted=0，ORDER BY last_time DESC）。
func (r *conversationRepo) ListByUser(ctx context.Context, userID string, limit int) ([]model.ConversationDO, error) {
	var dos []model.ConversationDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("user_id = ?", userID).
		Order("last_time DESC").Limit(limit).
		Find(&dos).Error; err != nil {
		return nil, fmt.Errorf("list conversations user_id=%s: %w", userID, err)
	}
	return dos, nil
}

// Exists 会话是否存在（不过滤 deleted，conversation_id 为 snowflake 全局唯一）。
func (r *conversationRepo) Exists(ctx context.Context, id string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Where("conversation_id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count conversation id=%s: %w", id, err)
	}
	return count > 0, nil
}

// ExistsForUser 会话是否存在且归属于指定用户（对照 createOrUpdateConversation：COUNT WHERE conversation_id=? AND user_id=?）。
func (r *conversationRepo) ExistsForUser(ctx context.Context, id, userID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Where("conversation_id = ? AND user_id = ?", id, userID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count conversation id=%s user_id=%s: %w", id, userID, err)
	}
	return count > 0, nil
}

func (r *conversationRepo) Create(ctx context.Context, c *model.ConversationDO) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	return nil
}

func (r *conversationRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Scopes(notDeleted).Where("conversation_id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update conversation id=%s: %w", id, err)
	}
	return nil
}

// UpdateFieldsForUser 更新会话字段（对照 RenameSession：WHERE conversation_id=? AND user_id=? AND deleted=0）。
func (r *conversationRepo) UpdateFieldsForUser(ctx context.Context, id, userID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Scopes(notDeleted).Where("conversation_id = ? AND user_id = ?", id, userID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update conversation id=%s user_id=%s: %w", id, userID, err)
	}
	return nil
}

func (r *conversationRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Where("conversation_id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete conversation id=%s: %w", id, err)
	}
	return nil
}

// SoftDeleteForUser 软删会话（对照 DeleteSession：WHERE conversation_id=? AND user_id=?）。
func (r *conversationRepo) SoftDeleteForUser(ctx context.Context, id, userID string) error {
	if err := r.db.WithContext(ctx).Model(&model.ConversationDO{}).
		Where("conversation_id = ? AND user_id = ?", id, userID).
		Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete conversation id=%s user_id=%s: %w", id, userID, err)
	}
	return nil
}
