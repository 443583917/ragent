package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// TermMappingRepository 同义词映射表数据访问。
type TermMappingRepository interface {
	ListEnabled(ctx context.Context) ([]model.TermMappingDO, error)
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.TermMappingDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.TermMappingDO, error)
	Create(ctx context.Context, m *model.TermMappingDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}
