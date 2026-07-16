package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// UserRepository 用户表数据访问。
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*model.UserDO, error)
	FindByUsername(ctx context.Context, username string) (*model.UserDO, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error)
	Create(ctx context.Context, u *model.UserDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}
