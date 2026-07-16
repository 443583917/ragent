package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// AuditLogRepository 业务变更审计日志表数据访问。
type AuditLogRepository interface {
	List(ctx context.Context, q model.PageQuery, f model.AuditLogFilter) ([]model.BizChangeLogDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.BizChangeLogDO, error)
	Create(ctx context.Context, l *model.BizChangeLogDO) error
}
