package admin

import (
	"context"
	"strconv"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"go.uber.org/zap"
)

// TraceService RAG Trace 查询服务接口。
type TraceService interface {
	ListRuns(ctx context.Context, q model.PageQuery, traceID, convID string) ([]model.RagTraceRunVO, int64, error)
	GetDetail(ctx context.Context, runID string) (*model.RagTraceDetailVO, error)
	GetNodes(ctx context.Context, runID string) ([]model.RagTraceNodeVO, error)
}

type traceService struct {
	repo repository.TraceRepository
}

// NewTraceService 创建 Trace 服务。
func NewTraceService(repo repository.TraceRepository) TraceService {
	return &traceService{repo: repo}
}

func (s *traceService) ListRuns(ctx context.Context, q model.PageQuery, traceID, convID string) ([]model.RagTraceRunVO, int64, error) {
	filter := model.TraceRunFilter{TraceID: traceID, ConversationID: convID}
	dos, total, err := s.repo.ListRuns(ctx, q, filter)
	if err != nil {
		zap.L().Error("查询 Trace 列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.RagTraceRunVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.RunDOToVO(d))
	}
	if vos == nil {
		vos = []model.RagTraceRunVO{}
	}
	return vos, total, nil
}

func (s *traceService) GetDetail(ctx context.Context, runID string) (*model.RagTraceDetailVO, error) {
	if runID == "" {
		return nil, errs.Param("runID 不能为空")
	}

	run, err := s.repo.FindRun(ctx, runID)
	if err != nil {
		return nil, errs.NotFound("Trace 不存在")
	}

	// 修复：第二个查询（ListNodes）的 error 不再忽略
	nodeDOs, err := s.repo.ListNodes(ctx, runID)
	if err != nil {
		zap.L().Error("查询 Trace 节点失败", zap.String("run_id", runID), zap.Error(err))
		return nil, errs.WrapBusiness(err, "查询 Trace 节点失败")
	}

	nodeVOs := make([]model.RagTraceNodeVO, 0, len(nodeDOs))
	for _, n := range nodeDOs {
		nodeVOs = append(nodeVOs, model.RagTraceNodeVO{
			TraceID: n.RunID, NodeID: strconv.FormatInt(n.ID, 10),
			ParentNodeID: n.ParentNodeID, NodeType: n.NodeType, NodeName: n.NodeName,
			Status: "DONE", DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}
	if nodeVOs == nil {
		nodeVOs = []model.RagTraceNodeVO{}
	}

	return &model.RagTraceDetailVO{
		Run:   model.RunDOToVO(*run),
		Nodes: nodeVOs,
	}, nil
}

func (s *traceService) GetNodes(ctx context.Context, runID string) ([]model.RagTraceNodeVO, error) {
	nodeDOs, err := s.repo.ListNodes(ctx, runID)
	if err != nil {
		zap.L().Error("查询 Trace 节点失败", zap.Error(err))
		return nil, errs.WrapServer(err, "查询失败")
	}

	vos := make([]model.RagTraceNodeVO, 0, len(nodeDOs))
	for _, n := range nodeDOs {
		vos = append(vos, model.RagTraceNodeVO{
			TraceID: n.RunID, NodeID: strconv.FormatInt(n.ID, 10),
			ParentNodeID: n.ParentNodeID, NodeType: n.NodeType, NodeName: n.NodeName,
			Status: "DONE", DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}
	if vos == nil {
		vos = []model.RagTraceNodeVO{}
	}
	return vos, nil
}
