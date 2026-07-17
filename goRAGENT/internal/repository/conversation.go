package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// ConversationRepository 会话表数据访问（id 均指业务字段 conversation_id）。
type ConversationRepository interface {
	ListByUser(ctx context.Context, userID string, limit int) ([]model.ConversationDO, error)
	Exists(ctx context.Context, id string) (bool, error)
	ExistsForUser(ctx context.Context, id, userID string) (bool, error)
	Create(ctx context.Context, c *model.ConversationDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	UpdateFieldsForUser(ctx context.Context, id, userID string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
	SoftDeleteForUser(ctx context.Context, id, userID string) error
}

// MessageRepository 会话消息表数据访问。
type MessageRepository interface {
	ListByConversation(ctx context.Context, convID string) ([]model.ConversationMessageDO, error)
	ListByConversationForUser(ctx context.Context, convID, userID string) ([]model.ConversationMessageDO, error)
	ListRecent(ctx context.Context, convID string, limit int) ([]model.ConversationMessageDO, error)
	ListRecentForUser(ctx context.Context, convID, userID string, limit int) ([]model.ConversationMessageDO, error)
	ListRange(ctx context.Context, convID string, afterID, beforeID int64) ([]model.ConversationMessageDO, error)
	ListRangeForUser(ctx context.Context, convID, userID string, afterID, beforeID int64) ([]model.ConversationMessageDO, error)
	CountUserMessages(ctx context.Context, convID string) (int64, error)
	CountUserMessagesForUser(ctx context.Context, convID, userID string) (int64, error)
	Create(ctx context.Context, m *model.ConversationMessageDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	UpdateVoteForUser(ctx context.Context, id string, userID string, vote *int) error
}

// SummaryRepository 会话摘要表数据访问。
type SummaryRepository interface {
	Latest(ctx context.Context, convID string) (*model.ConversationSummaryDO, error)
	LatestForUser(ctx context.Context, convID, userID string) (*model.ConversationSummaryDO, error)
	Create(ctx context.Context, s *model.ConversationSummaryDO) error
}
