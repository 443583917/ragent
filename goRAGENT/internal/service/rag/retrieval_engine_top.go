package rag

import (
	"context"

	"goRAGENT/internal/config"
	"goRAGENT/internal/model"
	"goRAGENT/pkg/prompt"
)

type RetrievalEngine struct {
	cfg     config.RAGConfig
	engine  *MultiChannelEngine
	prompts *prompt.TemplateLoader
}

type RetrievalContext struct {
	KbContext  string
	McpContext string
	IsEmpty    bool
}

func NewRetrievalEngine(cfg config.RAGConfig, engine *MultiChannelEngine, prompts *prompt.TemplateLoader) *RetrievalEngine {
	return &RetrievalEngine{cfg: cfg, engine: engine, prompts: prompts}
}

func (r *RetrievalEngine) Search(ctx context.Context, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	return r.engine.RetrieveKnowledgeChannels(ctx, sc)
}
