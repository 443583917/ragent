package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// TraceRepository RAG Trace 运行/节点表数据访问。
type TraceRepository interface {
	CreateRun(ctx context.Context, run *model.TraceRunDO) error
	UpdateRunFieldsByTaskID(ctx context.Context, taskID string, updates map[string]any) error
	ListRuns(ctx context.Context, q model.PageQuery, f model.TraceRunFilter) ([]model.TraceRunDO, int64, error)
	FindRun(ctx context.Context, runID string) (*model.TraceRunDO, error)
	ListNodes(ctx context.Context, runID string) ([]model.TraceNodeDO, error)
}
