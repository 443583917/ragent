package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// SampleQuestionRepository 示例问题表数据访问。
type SampleQuestionRepository interface {
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.SampleQuestionDO, int64, error)
	ListPublic(ctx context.Context, limit int) ([]model.SampleQuestionDO, error) // deleted=0 AND enabled=1
	Create(ctx context.Context, s *model.SampleQuestionDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}
