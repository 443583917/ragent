package rag

import (
	"context"
	"sort"

	"goRAGENT/internal/model"
)

// PostProcessor 后处理器接口（和 Java SearchResultPostProcessor 一致）
type PostProcessor interface {
	Name() string
	Order() int
	IsEnabled(ctx context.Context, sc *model.SearchContext) bool
	Process(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) ([]model.RetrievedChunk, error)
}

// ========== DedupPostProcessor 去重处理器 ==========

// DedupPostProcessor 按 ID 去重，保留高优先级通道的结果
type DedupPostProcessor struct{}

func (d *DedupPostProcessor) Name() string    { return "Deduplication" }
func (d *DedupPostProcessor) Order() int       { return 1 }

func (d *DedupPostProcessor) IsEnabled(ctx context.Context, sc *model.SearchContext) bool {
	return true
}

func (d *DedupPostProcessor) Process(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	seen := make(map[string]bool)
	var deduped []model.RetrievedChunk
	for _, c := range chunks {
		if !seen[c.ID] {
			seen[c.ID] = true
			deduped = append(deduped, c)
		}
	}
	return deduped, nil
}

// ========== FusionPostProcessor RRF 融合处理器 ==========

// FusionPostProcessor RRF（Reciprocal Rank Fusion）融合处理器
type FusionPostProcessor struct {
	RRFK               int  // RRF 平滑常数 k，默认 60
	RerankCandidateLimit int // 送入 Rerank 的候选上限（<=0 表示不截断）
}

func (f *FusionPostProcessor) Name() string  { return "Fusion(RRF)" }
func (f *FusionPostProcessor) Order() int    { return 5 }

func (f *FusionPostProcessor) IsEnabled(ctx context.Context, sc *model.SearchContext) bool {
	return true
}

func (f *FusionPostProcessor) Process(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	k := f.RRFK
	if k <= 0 {
		k = 60
	}

	// 对每个 chunk 按原始分数排名（相同 ID 取最高分）
	type rankedChunk struct {
		chunk model.RetrievedChunk
		rrf   float64
	}

	// 按分数降序排列
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})

	// 计算 RRF 分数：1 / (k + rank)
	var ranked []rankedChunk
	seen := make(map[string]bool)
	for i, c := range chunks {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		rrfScore := 1.0 / float64(k+i+1) // rank 从 1 开始
		ranked = append(ranked, rankedChunk{chunk: c, rrf: rrfScore})
	}

	// 按 RRF 分数降序排列
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].rrf > ranked[j].rrf
	})

	// 截断候选
	if f.RerankCandidateLimit > 0 && len(ranked) > f.RerankCandidateLimit {
		ranked = ranked[:f.RerankCandidateLimit]
	}

	var result []model.RetrievedChunk
	for _, rc := range ranked {
		result = append(result, rc.chunk)
	}
	return result, nil
}

// ========== RerankPostProcessor 重排序处理器 ==========

// Reranker Rerank 服务接口
type Reranker interface {
	Rerank(ctx context.Context, query string, documents []string) ([]string, []float64, error)
}

// RerankerAdapter 将 rerank 模块的 Service 适配为 retrieve.Reranker
func RerankerAdapter(r Reranker) Reranker { return r }

// RerankPostProcessor Rerank 重排序处理器
type RerankPostProcessor struct {
	reranker Reranker
	enabled  bool
}

func NewRerankPostProcessor(reranker Reranker, enabled bool) *RerankPostProcessor {
	return &RerankPostProcessor{reranker: reranker, enabled: enabled}
}

func (r *RerankPostProcessor) Name() string { return "Rerank" }
func (r *RerankPostProcessor) Order() int   { return 10 }

func (r *RerankPostProcessor) IsEnabled(ctx context.Context, sc *model.SearchContext) bool {
	return r.enabled && r.reranker != nil
}

func (r *RerankPostProcessor) Process(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	if len(chunks) == 0 || r.reranker == nil {
		return chunks, nil
	}

	// 提取文档文本
	docs := make([]string, len(chunks))
	for i, c := range chunks {
		docs[i] = c.Text
	}

	// 调用 Rerank API (返回 rankedDocs, scores, error)
	_, scores, err := r.reranker.Rerank(ctx, sc.RewrittenQuestion, docs)
	if err != nil {
		return chunks, err
	}

	for i := range chunks {
		if i < len(scores) {
			chunks[i].Score = scores[i]
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})

	return chunks, nil
}
