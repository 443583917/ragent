package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
	"go.uber.org/zap"
)

// IntentService 意图树管理服务接口。
type IntentService interface {
	GetTrees(ctx context.Context) ([]*model.IntentNodeTreeVO, error)
	Create(ctx context.Context, req model.IntentNodeCreateReq, operator string) (string, error)
	Update(ctx context.Context, id string, req model.IntentNodeUpdateReq, operator string) error
	Delete(ctx context.Context, id string, operator string) error
	BatchUpdate(ctx context.Context, ids []string, updates map[string]any, operator string) error
}

type intentService struct {
	repo         repository.IntentNodeRepository
	cacheClearer CacheClearer // 可为 nil
}

// NewIntentService 创建意图树服务。
func NewIntentService(repo repository.IntentNodeRepository, cc CacheClearer) IntentService {
	return &intentService{repo: repo, cacheClearer: cc}
}

func (s *intentService) clearCache(ctx context.Context) {
	if s.cacheClearer != nil {
		s.cacheClearer.ClearCache(ctx)
	}
}

func (s *intentService) GetTrees(ctx context.Context) ([]*model.IntentNodeTreeVO, error) {
	dos, err := s.repo.ListAll(ctx)
	if err != nil {
		zap.L().Error("查询意图树失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询意图树失败")
	}
	vos := model.BuildIntentTreeVOs(dos)
	if vos == nil {
		vos = []*model.IntentNodeTreeVO{}
	}
	return vos, nil
}

func (s *intentService) Create(ctx context.Context, req model.IntentNodeCreateReq, operator string) (string, error) {
	id := snowflake.NextID()
	do := model.IntentCreateReqToDO(req, id, operator)
	if err := s.repo.Create(ctx, &do); err != nil {
		zap.L().Error("创建意图节点失败", zap.Error(err))
		return "", errs.Business("创建失败（intentCode 可能重复）")
	}
	s.clearCache(ctx)
	return do.ID, nil
}

func (s *intentService) Update(ctx context.Context, id string, req model.IntentNodeUpdateReq, operator string) error {
	updates := model.IntentUpdateReqToUpdates(req, operator)
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新意图节点失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	s.clearCache(ctx)
	return nil
}

func (s *intentService) Delete(ctx context.Context, id string, operator string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除意图节点失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	s.clearCache(ctx)
	return nil
}

func (s *intentService) BatchUpdate(ctx context.Context, ids []string, updates map[string]any, operator string) error {
	if len(ids) == 0 {
		return nil
	}
	updates["update_by"] = operator
	if err := s.repo.BatchUpdateFields(ctx, ids, updates); err != nil {
		zap.L().Error("批量更新意图节点失败", zap.Error(err))
		return errs.WrapBusiness(err, "批量操作失败")
	}
	s.clearCache(ctx)
	return nil
}
