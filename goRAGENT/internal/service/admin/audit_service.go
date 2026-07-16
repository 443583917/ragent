package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"go.uber.org/zap"
)

// AuditService 审计日志服务接口。
type AuditService interface {
	List(ctx context.Context, q model.PageQuery, entityType string) ([]model.BizChangeLogVO, int64, error)
	Get(ctx context.Context, id string) (*model.BizChangeLogVO, error)
	// Write 写入审计日志（修复：传入 context，通过 repo WithContext 写入）。
	Write(ctx context.Context, entityType, entityID, action, operator, before, after string) error
}

type auditService struct {
	repo repository.AuditLogRepository
}

// NewAuditService 创建审计日志服务。
func NewAuditService(repo repository.AuditLogRepository) AuditService {
	return &auditService{repo: repo}
}

func (s *auditService) List(ctx context.Context, q model.PageQuery, entityType string) ([]model.BizChangeLogVO, int64, error) {
	filter := model.AuditLogFilter{EntityType: entityType}
	dos, total, err := s.repo.List(ctx, q, filter)
	if err != nil {
		zap.L().Error("查询审计日志失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.BizChangeLogVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.BizChangeLogVO{
			ID: d.ID, EntityType: d.EntityType, EntityID: d.EntityID,
			Action: d.Action, Operator: d.Operator,
			BeforeSnapshot: d.BeforeSnapshot, AfterSnapshot: d.AfterSnapshot,
			CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}
	return vos, total, nil
}

func (s *auditService) Get(ctx context.Context, id string) (*model.BizChangeLogVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("日志不存在")
	}
	return &model.BizChangeLogVO{
		ID: do.ID, EntityType: do.EntityType, EntityID: do.EntityID,
		Action: do.Action, Operator: do.Operator,
		BeforeSnapshot: do.BeforeSnapshot, AfterSnapshot: do.AfterSnapshot,
		CreateTime: do.CreateTime.Format("2006-01-02 15:04:05"),
	}, nil
}

func (s *auditService) Write(ctx context.Context, entityType, entityID, action, operator, before, after string) error {
	log := model.BizChangeLogDO{
		EntityType: entityType, EntityID: entityID, Action: action,
		Operator: operator, BeforeSnapshot: before, AfterSnapshot: after,
	}
	if err := s.repo.Create(ctx, &log); err != nil {
		zap.L().Warn("审计日志写入失败", zap.Error(err))
		return errs.WrapServer(err, "审计日志写入失败")
	}
	return nil
}
