package retrieve

import (
	"context"

	"goRAGENT/internal/config"
	"goRAGENT/internal/rag/prompt"
)

type RetrievalEngine struct {
	cfg          config.RAGConfig
	multiChannel *MultiChannelEngine
	prompts      *prompt.TemplateLoader
}

type RetrievalContext struct {
	KbContext  string
	McpContext string
	IsEmpty    bool
}

func NewRetrievalEngine(cfg config.RAGConfig, mce *MultiChannelEngine, p *prompt.TemplateLoader) *RetrievalEngine {
	return &RetrievalEngine{cfg: cfg, multiChannel: mce, prompts: p}
}

// Search 执行检索（简化版：直接走多通道引擎）
func (e *RetrievalEngine) Search(ctx context.Context, sc *SearchContext) ([]RetrievedChunk, error) {
	return e.multiChannel.RetrieveKnowledgeChannels(ctx, sc)
}
