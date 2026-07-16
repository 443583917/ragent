package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"go.uber.org/zap"
)

// IngestionTaskService 入库任务管理服务接口。
type IngestionTaskService interface {
	List(ctx context.Context, q model.PageQuery) ([]model.IngestionTaskVO, int64, error)
	Get(ctx context.Context, id string) (*model.IngestionTaskVO, error)
	GetNodes(ctx context.Context, id string) ([]model.IngestionNodeVO, error)
}

type ingestionTaskService struct {
	repo repository.IngestionTaskRepository
}

// NewIngestionTaskService 创建入库任务服务。
func NewIngestionTaskService(repo repository.IngestionTaskRepository) IngestionTaskService {
	return &ingestionTaskService{repo: repo}
}

func (s *ingestionTaskService) List(ctx context.Context, q model.PageQuery) ([]model.IngestionTaskVO, int64, error) {
	dos, total, err := s.repo.List(ctx, q)
	if err != nil {
		zap.L().Error("查询入库任务列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.IngestionTaskVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.TaskDOToVO(d))
	}
	return vos, total, nil
}

func (s *ingestionTaskService) Get(ctx context.Context, id string) (*model.IngestionTaskVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.Business("任务不存在")
	}
	vo := model.TaskDOToVO(*do)
	return &vo, nil
}

func (s *ingestionTaskService) GetNodes(ctx context.Context, id string) ([]model.IngestionNodeVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.Business("任务不存在")
	}

	nodeNames := []string{"Fetcher", "Parser", "Chunker", "Indexer"}
	nodes := make([]model.IngestionNodeVO, 0, 4)
	for i, name := range nodeNames {
		status := "PENDING"
		switch do.Status {
		case model.TaskStatusDone:
			status = "DONE"
		case model.TaskStatusFailed:
			if do.CompletedChunks == 0 && i == 0 {
				status = "FAILED"
			} else if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else {
				status = "FAILED"
			}
		case model.TaskStatusRunning:
			if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else if do.CompletedChunks > 0 {
				status = "RUNNING"
			} else if i == 0 {
				status = "RUNNING"
			}
		}
		nodes = append(nodes, model.IngestionNodeVO{Name: name, Status: status})
	}
	return nodes, nil
}
