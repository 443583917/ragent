package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// IntentNodeRepository 意图节点表数据访问。
type IntentNodeRepository interface {
	ListActive(ctx context.Context) ([]model.IntentNodeDO, error) // deleted=0 AND enabled=1
	ListAll(ctx context.Context) ([]model.IntentNodeDO, error)    // deleted=0
	Create(ctx context.Context, n *model.IntentNodeDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
	BatchUpdateFields(ctx context.Context, ids []string, updates map[string]any) error
}
