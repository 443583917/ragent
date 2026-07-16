package intent

import (
	"context"
	"strings"
	"testing"

	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/retrieve"
)

func subIntent(sub string, scores ...retrieve.NodeScore) retrieve.SubQuestionIntent {
	return retrieve.SubQuestionIntent{SubQuestion: sub, NodeScores: scores}
}

func ns(id string, score float64) retrieve.NodeScore {
	return retrieve.NodeScore{Node: &retrieve.NodeRef{ID: id, IsKB: true}, Score: score}
}

// ========== capTotalIntents ==========

func TestCapTotalIntents_UnderLimitUnchanged(t *testing.T) {
	subs := []retrieve.SubQuestionIntent{
		subIntent("q1", ns("a", 0.9)),
		subIntent("q2", ns("b", 0.8)),
	}
	got := capTotalIntents(subs, 3)
	if len(got) != 2 || len(got[0].NodeScores) != 1 || len(got[1].NodeScores) != 1 {
		t.Fatalf("未超限应原样保留: %+v", got)
	}
}

func TestCapTotalIntents_GuaranteePerSubThenByScore(t *testing.T) {
	subs := []retrieve.SubQuestionIntent{
		subIntent("q1", ns("a", 0.9), ns("b", 0.8), ns("c", 0.7)),
		subIntent("q2", ns("d", 0.85)),
	}
	got := capTotalIntents(subs, 3)
	// 保底: q1→a(0.9), q2→d(0.85)；剩余配额 1 → 全局最高剩余 b(0.8)
	if len(got) != 2 {
		t.Fatalf("应保留 2 个子问题: %+v", got)
	}
	q1 := got[0]
	if len(q1.NodeScores) != 2 || q1.NodeScores[0].Node.ID != "a" || q1.NodeScores[1].Node.ID != "b" {
		t.Errorf("q1 应保留 a+b: %+v", q1.NodeScores)
	}
	q2 := got[1]
	if len(q2.NodeScores) != 1 || q2.NodeScores[0].Node.ID != "d" {
		t.Errorf("q2 应保底 d: %+v", q2.NodeScores)
	}
}

func TestCapTotalIntents_LowScoreSubStillGuaranteed(t *testing.T) {
	subs := []retrieve.SubQuestionIntent{
		subIntent("q1", ns("a", 0.9), ns("b", 0.88), ns("c", 0.87)),
		subIntent("q2", ns("d", 0.4)), // 低分子问题也要保底
	}
	got := capTotalIntents(subs, 3)
	var q2 *retrieve.SubQuestionIntent
	for i := range got {
		if got[i].SubQuestion == "q2" {
			q2 = &got[i]
		}
	}
	if q2 == nil || len(q2.NodeScores) != 1 || q2.NodeScores[0].Node.ID != "d" {
		t.Fatalf("低分子问题应保底 1 个意图: %+v", got)
	}
	total := 0
	for _, s := range got {
		total += len(s.NodeScores)
	}
	if total != 3 {
		t.Errorf("总意图数应为 3: %d", total)
	}
}

func TestCapTotalIntents_SkipsEmptySubQuestions(t *testing.T) {
	subs := []retrieve.SubQuestionIntent{
		subIntent("q1", ns("a", 0.9)),
		subIntent("q2"), // 无意图
	}
	got := capTotalIntents(subs, 3)
	if len(got) != 1 || got[0].SubQuestion != "q1" {
		t.Fatalf("无意图子问题应被剔除: %+v", got)
	}
}

// ========== ResolveAll 并行分类 ==========

// routingChat 按 user 消息内容路由不同回复
type routingChat struct {
	responses map[string]string // 子问题关键字 → LLM 回复
}

func (r *routingChat) Chat(_ context.Context, req llm.ChatRequest) (string, error) {
	user := req.Messages[len(req.Messages)-1].Content
	for key, resp := range r.responses {
		if strings.Contains(user, key) {
			return resp, nil
		}
	}
	return "[]", nil
}

func TestResolveAll_ClassifiesEachSubQuestion(t *testing.T) {
	chat := &routingChat{responses: map[string]string{
		"请假": `[{"id":"biz-hr","score":0.9}]`,
		"打印": `[{"id":"biz-it","score":0.8}]`,
	}}
	c := NewClassifier(NewTreeLoader(nil, nil), chat, nil)
	c.treeOverride = []*IntentNode{
		{ID: "biz-hr", Name: "人事", FullPath: "域 > 人事", Kind: KindKB},
		{ID: "biz-it", Name: "IT", FullPath: "域 > IT", Kind: KindKB},
	}
	r := NewResolver(c)

	got := r.ResolveAll(context.Background(), []string{"请假流程", "打印机换墨盒"})

	if len(got) != 2 {
		t.Fatalf("应返回 2 个子问题意图: %+v", got)
	}
	if got[0].SubQuestion != "请假流程" || got[0].NodeScores[0].Node.ID != "biz-hr" {
		t.Errorf("子问题 1 分类错误: %+v", got[0])
	}
	if got[1].SubQuestion != "打印机换墨盒" || got[1].NodeScores[0].Node.ID != "biz-it" {
		t.Errorf("子问题 2 分类错误: %+v", got[1])
	}
}

func TestResolveAll_EmptyQuestionsReturnsNil(t *testing.T) {
	c := NewClassifier(NewTreeLoader(nil, nil), &routingChat{}, nil)
	r := NewResolver(c)
	if got := r.ResolveAll(context.Background(), nil); len(got) != 0 {
		t.Fatalf("空输入应返回空: %+v", got)
	}
}
