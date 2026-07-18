package rag

import (
	"context"
	"sort"
	"sync"

	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

const (
	// IntentMinScore 意图最低分数阈值
	IntentMinScore = 0.35
	// MaxIntentCount 单次查询最多保留意图数
	MaxIntentCount = 3
)

// Resolver 意图解析器：分类 → 阈值过滤 + cap → 转换为检索层类型。
// Go 版暂无查询改写拆分，整个问题作为单一子问题（结构保留，便于后续接入）。
type Resolver struct {
	classifier *Classifier
}

func NewResolver(classifier *Classifier) *Resolver {
	return &Resolver{classifier: classifier}
}

// Resolve 解析单个问题意图（ResolveAll 的单问题便捷入口）
func (r *Resolver) Resolve(ctx context.Context, question string) []model.SubQuestionIntent {
	return r.ResolveAll(ctx, []string{question})
}

// ResolveAll 按子问题并行分类 + 全局 capTotalIntents 保底分配
// （意图解析）.。失败/无命中返回空。
func (r *Resolver) ResolveAll(ctx context.Context, subQuestions []string) []model.SubQuestionIntent {
	if len(subQuestions) == 0 {
		return nil
	}
	results := make([][]NodeScore, len(subQuestions))
	var wg sync.WaitGroup
	for i, q := range subQuestions {
		wg.Add(1)
		go func(idx int, question string) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					zap.L().Error("意图分类 panic", zap.Any("recover", rec))
				}
			}()
			results[idx] = filterAndCap(r.classifier.ClassifyTargets(ctx, question))
		}(i, q)
	}
	wg.Wait()

	subs := make([]model.SubQuestionIntent, 0, len(subQuestions))
	for i, q := range subQuestions {
		scores := results[i]
		if len(scores) == 0 {
			continue
		}
		nodeScores := make([]model.NodeScore, len(scores))
		for j, ns := range scores {
			nodeScores[j] = model.NodeScore{Node: toNodeRef(ns.Node), Score: ns.Score}
		}
		subs = append(subs, model.SubQuestionIntent{SubQuestion: q, NodeScores: nodeScores})
	}
	return capTotalIntents(subs, MaxIntentCount)
}

// capTotalIntents 全局意图数限制：
// 1) 展平候选按分数降序；2) 每个子问题保底 1 个最高分意图；
// 3) 剩余配额按分数从高到低分配；4) 按子问题顺序重建（无意图子问题剔除）。
func capTotalIntents(subs []model.SubQuestionIntent, max int) []model.SubQuestionIntent {
	total := 0
	for _, s := range subs {
		total += len(s.NodeScores)
	}
	if total <= max {
		// 仍需剔除空子问题
		kept := subs[:0:0]
		for _, s := range subs {
			if len(s.NodeScores) > 0 {
				kept = append(kept, s)
			}
		}
		return kept
	}

	type candidate struct {
		subIdx int
		ns     model.NodeScore
	}
	var cands []candidate
	for i, s := range subs {
		for _, n := range s.NodeScores {
			cands = append(cands, candidate{subIdx: i, ns: n})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].ns.Score > cands[j].ns.Score })

	picked := make([]bool, len(cands))
	guaranteed := make(map[int]bool, len(subs))
	count := 0
	// 保底：每个子问题取全局最高分的那个
	for i, c := range cands {
		if !guaranteed[c.subIdx] {
			picked[i] = true
			guaranteed[c.subIdx] = true
			count++
		}
	}
	// 剩余配额按分数分配
	quota := max - count
	for i := range cands {
		if quota <= 0 {
			break
		}
		if !picked[i] {
			picked[i] = true
			quota--
		}
	}

	// 重建：保持子问题顺序，组内按分数降序（cands 已降序）
	grouped := make(map[int][]model.NodeScore, len(subs))
	for i, c := range cands {
		if picked[i] {
			grouped[c.subIdx] = append(grouped[c.subIdx], c.ns)
		}
	}
	var result []model.SubQuestionIntent
	for i, s := range subs {
		if scores := grouped[i]; len(scores) > 0 {
			result = append(result, model.SubQuestionIntent{SubQuestion: s.SubQuestion, NodeScores: scores})
		}
	}
	return result
}

// filterAndCap score >= IntentMinScore 过滤 + 最多 MaxIntentCount 个
// （输入已按 score 降序，直接截断）
func filterAndCap(scores []NodeScore) []NodeScore {
	var kept []NodeScore
	for _, ns := range scores {
		if ns.Score < IntentMinScore {
			continue
		}
		kept = append(kept, ns)
		if len(kept) == MaxIntentCount {
			break
		}
	}
	return kept
}

// toNodeRef model.IntentNode → model.NodeRef（检索层轻量引用）
func toNodeRef(n *model.IntentNode) *model.NodeRef {
	return &model.NodeRef{
		ID:             n.ID,
		Name:           n.Name,
		FullPath:       n.FullPath,
		CollectionName: n.CollectionName,
		McpToolID:      n.McpToolID,
		PromptSnippet:  n.PromptSnippet,
		PromptTemplate: n.PromptTemplate,
		TopK:           n.TopK,
		IsKB:           n.Kind == model.KindKB,
		IsMCP:          n.Kind == model.KindMCP,
	}
}
