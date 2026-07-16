package guidance

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/llm"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
)

func kbNS(id, name, fullPath string, score float64) retrieve.NodeScore {
	return retrieve.NodeScore{
		Node:  &retrieve.NodeRef{ID: id, Name: name, FullPath: fullPath, IsKB: true},
		Score: score,
	}
}

func singleSub(scores ...retrieve.NodeScore) []retrieve.SubQuestionIntent {
	return []retrieve.SubQuestionIntent{{SubQuestion: "q", NodeScores: scores}}
}

type fakeChat struct {
	resp   string
	err    error
	called bool
}

func (f *fakeChat) Chat(_ context.Context, _ llm.ChatRequest) (string, error) {
	f.called = true
	return f.resp, f.err
}

func detector(chat *fakeChat) *Detector {
	cfg := config.GuidanceConfig{Enabled: true, AmbiguityScoreRatio: 0.8, AmbiguityMargin: 0.15, MaxOptions: 6}
	return NewDetector(cfg, chat, nil)
}

// ========== 纯逻辑 ==========

func TestRankCandidates_KBOnlyDedupByIDDesc(t *testing.T) {
	subs := singleSub(
		kbNS("a", "A", "域 > A", 0.7),
		kbNS("a", "A", "域 > A", 0.9), // 同 ID 取最高
		retrieve.NodeScore{Node: &retrieve.NodeRef{ID: "m", IsMCP: true}, Score: 0.95}, // 非 KB 过滤
		kbNS("b", "B", "域 > B", 0.8),
	)
	ranked := rankCandidates(subs)
	if len(ranked) != 2 {
		t.Fatalf("应聚合出 2 个候选: %+v", ranked)
	}
	if ranked[0].Node.ID != "a" || ranked[0].Score != 0.9 {
		t.Errorf("同 ID 应取最高分且降序: %+v", ranked[0])
	}
}

func TestRenderOptions_NumberedCapped(t *testing.T) {
	ranked := []retrieve.NodeScore{
		kbNS("a", "人事", "集团信息化 > 人事", 0.9),
		kbNS("b", "行政", "集团信息化 > 行政", 0.8),
	}
	got := renderOptions(ranked, 6)
	if !strings.Contains(got, "1) 集团信息化 > 人事") || !strings.Contains(got, "2) 集团信息化 > 行政") {
		t.Errorf("选项格式错误:\n%s", got)
	}
	capped := renderOptions([]retrieve.NodeScore{
		kbNS("a", "A", "A", 0.9), kbNS("b", "B", "B", 0.8), kbNS("c", "C", "C", 0.7),
	}, 2)
	if strings.Contains(capped, "3)") {
		t.Errorf("应按 maxOptions 截断:\n%s", capped)
	}
}

// ========== Detect 决策 ==========

func TestDetect_DisabledOrMultiSubReturnsEmpty(t *testing.T) {
	chat := &fakeChat{}
	d := detector(chat)
	d.cfg.Enabled = false
	if got := d.Detect(context.Background(), "问", singleSub(kbNS("a", "A", "A", 0.9), kbNS("b", "B", "B", 0.85))); got != "" {
		t.Errorf("关闭时应返回空: %q", got)
	}
	d2 := detector(chat)
	multi := []retrieve.SubQuestionIntent{
		{SubQuestion: "q1", NodeScores: []retrieve.NodeScore{kbNS("a", "A", "A", 0.9)}},
		{SubQuestion: "q2", NodeScores: []retrieve.NodeScore{kbNS("b", "B", "B", 0.85)}},
	}
	if got := d2.Detect(context.Background(), "问", multi); got != "" {
		t.Errorf("多子问题应返回空: %q", got)
	}
}

func TestDetect_ClearIntentNoGuidance(t *testing.T) {
	chat := &fakeChat{}
	d := detector(chat)
	// ratio = 0.5/0.9 ≈ 0.56 < 0.65 → 明确，不引导
	got := d.Detect(context.Background(), "请假流程",
		singleSub(kbNS("a", "人事", "域 > 人事", 0.9), kbNS("b", "行政", "域 > 行政", 0.5)))
	if got != "" {
		t.Errorf("意图明确不应引导: %q", got)
	}
	if chat.called {
		t.Error("明确场景不应调 LLM")
	}
}

func TestDetect_HighRatioDirectGuidance(t *testing.T) {
	chat := &fakeChat{}
	d := detector(chat)
	// ratio = 0.85/0.9 ≈ 0.944 ≥ 0.8 → 直接判歧义，不调 LLM
	got := d.Detect(context.Background(), "数据安全规范",
		singleSub(kbNS("a", "OA数据安全", "OA系统 > 数据安全", 0.9), kbNS("b", "保险数据安全", "保险系统 > 数据安全", 0.85)))
	if got == "" {
		t.Fatal("高比值应触发引导")
	}
	if !strings.Contains(got, "OA数据安全") || !strings.Contains(got, "1) OA系统 > 数据安全") {
		t.Errorf("引导话术应含主题和选项:\n%s", got)
	}
	if chat.called {
		t.Error("ratio≥0.8 不应调 LLM 二次确认")
	}
}

func TestDetect_BorderlineUsesLLMCheck(t *testing.T) {
	// ratio = 0.63/0.9 = 0.7 ∈ [0.65, 0.8) → LLM 二次确认
	subs := singleSub(kbNS("a", "A类", "域A > 分类", 0.9), kbNS("b", "B类", "域B > 分类", 0.63))

	confirm := &fakeChat{resp: `{"ambiguous": true, "reason": "语义相同"}`}
	if got := detector(confirm).Detect(context.Background(), "分类规则", subs); got == "" {
		t.Error("LLM 确认歧义应触发引导")
	}
	if !confirm.called {
		t.Error("边界区间应调 LLM")
	}

	deny := &fakeChat{resp: `{"ambiguous": false}`}
	if got := detector(deny).Detect(context.Background(), "分类规则", subs); got != "" {
		t.Errorf("LLM 否认歧义不应引导: %q", got)
	}

	// LLM 失败 → 保守判歧义
	failed := &fakeChat{err: fmt.Errorf("unavailable")}
	if got := detector(failed).Detect(context.Background(), "分类规则", subs); got == "" {
		t.Error("LLM 失败应保守触发引导")
	}
}

func TestDetect_QuestionContainsDomainSkips(t *testing.T) {
	chat := &fakeChat{}
	d := detector(chat)
	// 问题里明确提到 "OA系统"（候选 FullPath 首段）→ 跳过引导
	got := d.Detect(context.Background(), "OA系统的数据安全规范",
		singleSub(kbNS("a", "OA数据安全", "OA系统 > 数据安全", 0.9), kbNS("b", "保险数据安全", "保险系统 > 数据安全", 0.85)))
	if got != "" {
		t.Errorf("问题含域名线索应跳过引导: %q", got)
	}
}

func TestDetect_SingleCandidateNoGuidance(t *testing.T) {
	d := detector(&fakeChat{})
	if got := d.Detect(context.Background(), "问", singleSub(kbNS("a", "A", "A", 0.9))); got != "" {
		t.Errorf("单候选无歧义: %q", got)
	}
}
