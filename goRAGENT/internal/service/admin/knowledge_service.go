package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
	"go.uber.org/zap"
)

// VectorStore 向量库抽象（Milvus），KnowledgeBaseService 在创建/删除 KB 时操作。
type VectorStore interface {
	CreateCollection(ctx context.Context, name string, dimension int) error
	DropCollection(ctx context.Context, name string) error
}

// KnowledgeBaseService 知识库 CRUD 服务接口。
type KnowledgeBaseService interface {
	List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseVO, int64, error)
	Create(ctx context.Context, req model.KnowledgeBaseCreateReq) (*model.KnowledgeBaseVO, error)
	Get(ctx context.Context, id string) (*model.KnowledgeBaseVO, error)
	Update(ctx context.Context, id string, req model.KnowledgeBaseUpdateReq) error
	Delete(ctx context.Context, id string) error
}

type knowledgeBaseService struct {
	repo        repository.KnowledgeBaseRepository
	vectorStore VectorStore // 可为 nil
}

// NewKnowledgeBaseService 创建知识库服务。
func NewKnowledgeBaseService(repo repository.KnowledgeBaseRepository, vs VectorStore) KnowledgeBaseService {
	return &knowledgeBaseService{repo: repo, vectorStore: vs}
}

func (s *knowledgeBaseService) List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseVO, int64, error) {
	dos, total, err := s.repo.List(ctx, q)
	if err != nil {
		zap.L().Error("查询知识库列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.KnowledgeBaseVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.KnowledgeBaseDOToVO(d))
	}
	return vos, total, nil
}

func (s *knowledgeBaseService) Create(ctx context.Context, req model.KnowledgeBaseCreateReq) (*model.KnowledgeBaseVO, error) {
	id := snowflake.NextID()
	collectionName := model.KBCollectionPrefix + id

	if s.vectorStore != nil {
		if err := s.vectorStore.CreateCollection(ctx, collectionName, model.DefaultVectorDimension); err != nil {
			zap.L().Error("创建 Milvus Collection 失败", zap.Error(err))
			return nil, errs.Business("创建向量集合失败")
		}
	}

	do := model.KnowledgeBaseDO{
		ID: id, Name: req.Name, Description: req.Description,
		CollectionName: collectionName, Dimension: model.DefaultVectorDimension,
	}
	if err := s.repo.Create(ctx, &do); err != nil {
		zap.L().Error("创建知识库失败", zap.Error(err))
		return nil, errs.WrapBusiness(err, "创建失败")
	}

	vo := model.KnowledgeBaseDOToVO(do)
	return &vo, nil
}

func (s *knowledgeBaseService) Get(ctx context.Context, id string) (*model.KnowledgeBaseVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("知识库不存在")
	}
	vo := model.KnowledgeBaseDOToVO(*do)
	return &vo, nil
}

func (s *knowledgeBaseService) Update(ctx context.Context, id string, req model.KnowledgeBaseUpdateReq) error {
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新知识库失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	return nil
}

func (s *knowledgeBaseService) Delete(ctx context.Context, id string) error {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return errs.NotFound("知识库不存在")
	}

	if s.vectorStore != nil && do.CollectionName != "" {
		if err := s.vectorStore.DropCollection(ctx, do.CollectionName); err != nil {
			zap.L().Error("删除 Milvus Collection 失败", zap.Error(err))
			// 不阻断流程，继续软删除
		}
	}

	if err := s.repo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除知识库失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	return nil
}
