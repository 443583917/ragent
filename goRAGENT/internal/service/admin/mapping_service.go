package admin

import (
	"context"
	"strings"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
	"go.uber.org/zap"
)

// MappingService 关键词映射管理服务接口。
type MappingService interface {
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.MappingVO, int64, error)
	Get(ctx context.Context, id string) (*model.MappingVO, error)
	Create(ctx context.Context, req model.MappingCreateReq, operator string) (string, error)
	Update(ctx context.Context, id string, req model.MappingUpdateReq, operator string) error
	Delete(ctx context.Context, id string, operator string) error
}

type mappingService struct {
	repo         repository.TermMappingRepository
	cacheClearer CacheClearer // 可为 nil
}

// NewMappingService 创建关键词映射服务。
func NewMappingService(repo repository.TermMappingRepository, cc CacheClearer) MappingService {
	return &mappingService{repo: repo, cacheClearer: cc}
}

func (s *mappingService) clearCache(ctx context.Context) {
	if s.cacheClearer != nil {
		s.cacheClearer.ClearCache(ctx)
	}
}

func (s *mappingService) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.MappingVO, int64, error) {
	keyword = strings.TrimSpace(keyword)
	rows, total, err := s.repo.List(ctx, q.Normalize(), keyword)
	if err != nil {
		zap.L().Error("查询映射列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.MappingVO, len(rows))
	for i, r := range rows {
		vos[i] = model.MappingToVO(r)
	}
	return vos, total, nil
}

func (s *mappingService) Get(ctx context.Context, id string) (*model.MappingVO, error) {
	row, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("映射不存在")
	}
	vo := model.MappingToVO(*row)
	return &vo, nil
}

func (s *mappingService) Create(ctx context.Context, req model.MappingCreateReq, operator string) (string, error) {
	id := snowflake.NextID()
	do := model.MappingCreateReqToDO(req, id, operator)
	if err := s.repo.Create(ctx, &do); err != nil {
		zap.L().Error("创建映射失败", zap.Error(err))
		return "", errs.WrapBusiness(err, "创建失败")
	}
	s.clearCache(ctx)
	return do.ID, nil
}

func (s *mappingService) Update(ctx context.Context, id string, req model.MappingUpdateReq, operator string) error {
	updates := model.MappingUpdateReqToUpdates(req, operator)
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新映射失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	s.clearCache(ctx)
	return nil
}

func (s *mappingService) Delete(ctx context.Context, id string, operator string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除映射失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	s.clearCache(ctx)
	return nil
}
