package ingestion

import (
	"context"

	"goRAGENT/internal/model"
)

// Node 入库流水线节点接口
type Node interface {
	Name() string
	Execute(ctx context.Context, pc *PipelineContext) error
}

// PipelineContext 在节点间流转的上下文
type PipelineContext struct {
	Task     *model.IngestionTaskDO
	KB       *model.KnowledgeBaseDO
	Doc      *model.DocumentDO
	FilePath string          // Fetcher 设置的文件路径
	Markdown string          // Parser 产出
	Chunks   []ChunkSegment  // Chunker 产出
}

// ChunkSegment 切分后的文本段（未嵌入）
type ChunkSegment struct {
	Index     int
	Text      string
	CharCount int
}
