package rag

import (
	"context"
	"fmt"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// TraceRecorder 追踪记录器，封装 TraceRun 创建与状态流转。
// 由 chat handler 的 StreamChat / StopTask 使用，替换 raw gorm.DB 操作。
type TraceRecorder interface {
	// StartRun 创建一条 RUNNING 状态的追踪记录。
	StartRun(ctx context.Context, taskID, conversationID, userID, question string) error

	// FinishRun 按 taskID 更新追踪状态（成功/失败/空）。
	FinishRun(ctx context.Context, taskID string, status string) error

	// CancelByTaskID 按 taskID 取消追踪（CANCELLED + 错误信息）。
	CancelByTaskID(ctx context.Context, taskID string, errorMessage string) error
}

// TraceRecorder 错误信息常量。
const (
	TraceErrClientDisconnect = "客户端断连"
	TraceErrUserCancel       = "用户取消"
)

type traceRecorder struct {
	repo repository.TraceRepository
}

// NewTraceRecorder 创建 TraceRecorder 实现。
func NewTraceRecorder(repo repository.TraceRepository) TraceRecorder {
	return &traceRecorder{repo: repo}
}

func (r *traceRecorder) StartRun(ctx context.Context, taskID, conversationID, userID, question string) error {
	if err := r.repo.CreateRun(ctx, &model.TraceRunDO{
		RunID:          taskID,
		ConversationID: conversationID,
		UserID:         userID,
		Question:       question,
		Status:         model.TraceStatusRunning,
	}); err != nil {
		return fmt.Errorf("trace recorder start run: %w", err)
	}
	return nil
}

func (r *traceRecorder) FinishRun(ctx context.Context, taskID string, status string) error {
	if err := r.repo.UpdateRunFieldsByTaskID(ctx, taskID, map[string]any{
		"status": status,
	}); err != nil {
		return fmt.Errorf("trace recorder finish run: %w", err)
	}
	return nil
}

func (r *traceRecorder) CancelByTaskID(ctx context.Context, taskID string, errorMessage string) error {
	if err := r.repo.UpdateRunFieldsByTaskID(ctx, taskID, map[string]any{
		"status":        model.TraceStatusCancelled,
		"error_message": errorMessage,
	}); err != nil {
		return fmt.Errorf("trace recorder cancel run: %w", err)
	}
	return nil
}
