package rag

import (
	"context"
	"fmt"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"go.uber.org/zap"
)

const timeLayout = "2006-01-02 15:04:05"

// ConversationVO 会话 VO（和前端 sessionService.ConversationVO 一致）
type ConversationVO struct {
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	LastTime       string `json:"lastTime,omitempty"`
}

// ConversationMessageVO 消息 VO（和前端 ConversationMessageVO 一致）
type ConversationMessageVO struct {
	ID               int64  `json:"id"`
	ConversationID   string `json:"conversationId"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	ThinkingContent  string `json:"thinkingContent,omitempty"`
	ThinkingDuration *int   `json:"thinkingDuration"`
	Vote             *int   `json:"vote"`
	CreateTime       string `json:"createTime,omitempty"`
}

func convToVO(d model.ConversationDO) ConversationVO {
	vo := ConversationVO{ConversationID: d.ConversationID, Title: d.Title}
	if !d.LastTime.IsZero() {
		vo.LastTime = d.LastTime.Format(timeLayout)
	}
	return vo
}

func msgToVO(d model.ConversationMessageDO) ConversationMessageVO {
	vo := ConversationMessageVO{
		ID: d.ID, ConversationID: d.ConversationID, Role: d.Role, Content: d.Content,
		ThinkingContent: d.ThinkingContent, ThinkingDuration: d.ThinkingDuration, Vote: d.Vote,
	}
	if !d.CreateTime.IsZero() {
		vo.CreateTime = d.CreateTime.Format(timeLayout)
	}
	return vo
}

// SessionService 会话业务接口（无 HTTP 依赖）。
type SessionService interface {
	ListSessions(ctx context.Context, userID string) ([]ConversationVO, error)
	RenameSession(ctx context.Context, conversationID, userID, title string) error
	DeleteSession(ctx context.Context, conversationID, userID string) error
	ListMessages(ctx context.Context, conversationID, userID string) ([]ConversationMessageVO, error)
	SubmitFeedback(ctx context.Context, messageID, userID string, vote *int) error
	CancelFeedback(ctx context.Context, messageID, userID string) error
}

type sessionService struct {
	convRepo repository.ConversationRepository
	msgRepo  repository.MessageRepository
}

// NewSessionService 创建 SessionService 实现。
func NewSessionService(convRepo repository.ConversationRepository, msgRepo repository.MessageRepository) SessionService {
	return &sessionService{convRepo: convRepo, msgRepo: msgRepo}
}

func (s *sessionService) ListSessions(ctx context.Context, userID string) ([]ConversationVO, error) {
	rows, err := s.convRepo.ListByUser(ctx, userID, 200)
	if err != nil {
		zap.L().Error("查询会话列表失败", zap.Error(err))
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	vos := make([]ConversationVO, len(rows))
	for i, r := range rows {
		vos[i] = convToVO(r)
	}
	return vos, nil
}

func (s *sessionService) RenameSession(ctx context.Context, conversationID, userID, title string) error {
	if err := s.convRepo.UpdateFieldsForUser(ctx, conversationID, userID, map[string]any{"title": title}); err != nil {
		zap.L().Error("重命名会话失败", zap.Error(err))
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}

func (s *sessionService) DeleteSession(ctx context.Context, conversationID, userID string) error {
	if err := s.convRepo.SoftDeleteForUser(ctx, conversationID, userID); err != nil {
		zap.L().Error("删除会话失败", zap.Error(err))
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *sessionService) ListMessages(ctx context.Context, conversationID, userID string) ([]ConversationMessageVO, error) {
	rows, err := s.msgRepo.ListByConversationForUser(ctx, conversationID, userID)
	if err != nil {
		zap.L().Error("查询会话消息失败", zap.Error(err))
		return nil, fmt.Errorf("list messages: %w", err)
	}
	vos := make([]ConversationMessageVO, len(rows))
	for i, r := range rows {
		vos[i] = msgToVO(r)
	}
	return vos, nil
}

// SubmitFeedback 提交/更新反馈（vote=nil 等效取消，通过 UpdateVoteForUser 实现）。
func (s *sessionService) SubmitFeedback(ctx context.Context, messageID, userID string, vote *int) error {
	if err := s.msgRepo.UpdateVoteForUser(ctx, messageID, userID, vote); err != nil {
		zap.L().Error("更新反馈失败", zap.Error(err))
		return fmt.Errorf("submit feedback: %w", err)
	}
	return nil
}

// CancelFeedback 取消反馈（vote=nil）。
func (s *sessionService) CancelFeedback(ctx context.Context, messageID, userID string) error {
	if err := s.msgRepo.UpdateVoteForUser(ctx, messageID, userID, nil); err != nil {
		zap.L().Error("取消反馈失败", zap.Error(err))
		return fmt.Errorf("cancel feedback: %w", err)
	}
	return nil
}
