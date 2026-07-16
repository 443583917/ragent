package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// auditLogRepo AuditLogRepository 的 GORM 实现（审计表无 deleted 列）。
type auditLogRepo struct{ db *gorm.DB }

// NewAuditLogRepo 创建审计日志 repository。
func NewAuditLogRepo(db *gorm.DB) repository.AuditLogRepository {
	return &auditLogRepo{db: db}
}

// List 分页查询（对照 listBizChangeLogs：entityType 精确匹配，ORDER BY create_time DESC）。
func (r *auditLogRepo) List(ctx context.Context, q model.PageQuery, f model.AuditLogFilter) ([]model.BizChangeLogDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.BizChangeLogDO{})
	if f.EntityType != "" {
		tx = tx.Where("entity_type = ?", f.EntityType)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}
	var dos []model.BizChangeLogDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	return dos, total, nil
}

func (r *auditLogRepo) FindByID(ctx context.Context, id string) (*model.BizChangeLogDO, error) {
	var do model.BizChangeLogDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find audit log id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *auditLogRepo) Create(ctx context.Context, l *model.BizChangeLogDO) error {
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}
