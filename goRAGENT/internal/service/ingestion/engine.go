package ingestion

import (
	"context"
	"strconv"
	"time"

	"goRAGENT/internal/config"
	"goRAGENT/pkg/embedding"
	"goRAGENT/pkg/mineru"
	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/milvus"
	"go.uber.org/zap"
)

// Engine 入库流水线引擎
type Engine struct {
	taskRepo repository.IngestionTaskRepository
	docRepo  repository.DocumentRepository
	kbRepo   repository.KnowledgeBaseRepository
	nodes    []Node
}

// NewEngine 创建引擎（注入依赖 + 组装节点链）
func NewEngine(taskRepo repository.IngestionTaskRepository, docRepo repository.DocumentRepository,
	kbRepo repository.KnowledgeBaseRepository, chunkRepo repository.ChunkRepository,
	dataDir string, mineruClient *mineru.Client, embedSvc *embedding.Service,
	milvusStore *milvus.MilvusStore, cfg config.IngestionConfig) *Engine {
	if dataDir == "" {
		dataDir = "data"
	}

	return &Engine{
		taskRepo: taskRepo,
		docRepo:  docRepo,
		kbRepo:   kbRepo,
		nodes: []Node{
			&Fetcher{DataDir: dataDir},
			NewParser(mineruClient, dataDir),
			NewChunker(cfg),
			NewIndexer(chunkRepo, taskRepo, docRepo, embedSvc, milvusStore, cfg.EmbedBatchSize),
		},
	}
}

// Run 异步执行入库 Pipeline
func (e *Engine) Run(taskID int64) {
	go func() {
		ctx := context.Background()

		// 1. 加载 task + doc + kb
		task, err := e.taskRepo.FindByID(ctx, strconv.FormatInt(taskID, 10))
		if err != nil {
			zap.L().Error("查询入库任务失败", zap.Int64("task_id", taskID), zap.Error(err))
			return
		}

		doc, err := e.docRepo.FindByID(ctx, task.DocID)
		if err != nil {
			zap.L().Error("查询文档失败", zap.String("doc_id", task.DocID), zap.Error(err))
			return
		}

		kb, err := e.kbRepo.FindByID(ctx, task.KbID)
		if err != nil {
			zap.L().Error("查询知识库失败", zap.String("kb_id", task.KbID), zap.Error(err))
			return
		}

		// 2. 更新状态为 RUNNING
		e.taskRepo.UpdateFields(ctx, strconv.FormatInt(task.ID, 10), map[string]any{"status": model.TaskStatusRunning})
		e.docRepo.UpdateFields(ctx, doc.ID, map[string]any{"status": model.DocStatusProcessing})

		// 3. 构建上下文 + 顺序执行节点
		pc := &PipelineContext{Task: task, KB: kb, Doc: doc}

		for _, node := range e.nodes {
			t0 := time.Now()
			zap.L().Info("入库节点开始", zap.String("node", node.Name()), zap.Int64("task_id", taskID))

			if err := node.Execute(ctx, pc); err != nil {
				zap.L().Error("入库节点失败",
					zap.String("node", node.Name()),
					zap.Int64("task_id", taskID),
					zap.Error(err),
				)
				e.taskRepo.UpdateFields(ctx, strconv.FormatInt(task.ID, 10), map[string]any{
					"status":        model.TaskStatusFailed,
					"error_message": err.Error(),
				})
				e.docRepo.UpdateFields(ctx, doc.ID, map[string]any{"status": model.DocStatusFailed})
				return
			}

			zap.L().Info("入库节点完成",
				zap.String("node", node.Name()),
				zap.Duration("latency", time.Since(t0)),
			)
		}

		// 4. 全部成功
		e.taskRepo.UpdateFields(ctx, strconv.FormatInt(task.ID, 10), map[string]any{
			"status":       model.TaskStatusDone,
			"total_chunks": len(pc.Chunks),
		})

		zap.L().Info("入库 Pipeline 完成",
			zap.Int64("task_id", taskID),
			zap.String("doc_id", doc.ID),
			zap.Int("chunks", len(pc.Chunks)),
		)
	}()
}
