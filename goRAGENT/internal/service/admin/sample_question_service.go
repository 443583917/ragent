package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
	"go.uber.org/zap"
)

// SampleQuestionService 示例问题管理服务接口。
type SampleQuestionService interface {
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.SampleQuestionItemVO, int64, error)
	ListPublic(ctx context.Context) ([]model.SampleQuestionItemVO, error)
	Create(ctx context.Context, req model.SampleQuestionPayload) (string, error)
	Update(ctx context.Context, id string, req model.SampleQuestionPayload) error
	Delete(ctx context.Context, id string) error
}

type sampleQuestionService struct {
	repo repository.SampleQuestionRepository
}

// NewSampleQuestionService 创建示例问题服务。
func NewSampleQuestionService(repo repository.SampleQuestionRepository) SampleQuestionService {
	return &sampleQuestionService{repo: repo}
}

func (s *sampleQuestionService) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.SampleQuestionItemVO, int64, error) {
	dos, total, err := s.repo.List(ctx, q, keyword)
	if err != nil {
		zap.L().Error("查询示例问题失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.SampleQuestionItemVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.SQDOToItem(d))
	}
	if vos == nil {
		vos = []model.SampleQuestionItemVO{}
	}
	return vos, total, nil
}

func (s *sampleQuestionService) ListPublic(ctx context.Context) ([]model.SampleQuestionItemVO, error) {
	// 修复：处理之前被忽略的 error
	dos, err := s.repo.ListPublic(ctx, 10)
	if err != nil {
		zap.L().Error("查询公开示例问题失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询失败")
	}

	vos := make([]model.SampleQuestionItemVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.SQDOToItem(d))
	}
	if vos == nil {
		vos = []model.SampleQuestionItemVO{}
	}
	return vos, nil
}

func (s *sampleQuestionService) Create(ctx context.Context, req model.SampleQuestionPayload) (string, error) {
	if req.Question == nil || *req.Question == "" {
		return "", errs.Param("question 不能为空")
	}

	title := ""
	if req.Title != nil {
		title = *req.Title
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}

	do := model.SampleQuestionDO{
		ID: snowflake.NextID(), Title: title, Description: desc,
		Question: *req.Question, SortOrder: 0, Enabled: 1,
	}
	if err := s.repo.Create(ctx, &do); err != nil {
		zap.L().Error("创建示例问题失败", zap.Error(err))
		return "", errs.WrapBusiness(err, "创建失败")
	}
	return do.ID, nil
}

func (s *sampleQuestionService) Update(ctx context.Context, id string, req model.SampleQuestionPayload) error {
	updates := map[string]any{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Question != nil {
		updates["question"] = *req.Question
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新示例问题失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	return nil
}

func (s *sampleQuestionService) Delete(ctx context.Context, id string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除示例问题失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	return nil
}
