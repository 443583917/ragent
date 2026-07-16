package rewrite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"goRAGENT/internal/infra/llm"
)

// ========== 规则拆分 fallback ==========

func TestRuleBasedSplit_MultiDelimiters(t *testing.T) {
	got := ruleBasedSplit("A系统怎么用？B系统呢；C流程。")
	want := []string{"A系统怎么用？", "B系统呢？", "C流程？"}
	if len(got) != 3 {
		t.Fatalf("应拆成 3 个: %v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("第 %d 个应为 %q 实际 %q", i, w, got[i])
		}
	}
}

func TestRuleBasedSplit_SingleQuestion(t *testing.T) {
	got := ruleBasedSplit("只有一个问题")
	if len(got) != 1 || got[0] != "只有一个问题？" {
		t.Errorf("单问题应返回 1 条且补问号: %v", got)
	}
}

func TestRuleBasedSplit_FiltersEmpty(t *testing.T) {
	got := ruleBasedSplit("？？A？？")
	if len(got) != 1 || got[0] != "A？" {
		t.Errorf("空片段应过滤: %v", got)
	}
}

// ========== LLM 输出解析 ==========

func TestParseRewrite_Normal(t *testing.T) {
	raw := `{"rewrite":"12306的架构","should_split":true,"sub_questions":["订单流程","支付处理"]}`
	rr := parseRewrite(raw)
	if rr == nil || rr.Rewritten != "12306的架构" {
		t.Fatalf("解析失败: %+v", rr)
	}
	if len(rr.SubQuestions) != 2 {
		t.Errorf("子问题应 2 条: %v", rr.SubQuestions)
	}
}

func TestParseRewrite_FenceAndEmptySubQuestions(t *testing.T) {
	raw := "```json\n{\"rewrite\":\"OA架构\",\"should_split\":false}\n```"
	rr := parseRewrite(raw)
	if rr == nil || rr.Rewritten != "OA架构" {
		t.Fatalf("应剥离 fence: %+v", rr)
	}
	if len(rr.SubQuestions) != 1 || rr.SubQuestions[0] != "OA架构" {
		t.Errorf("sub_questions 缺省应为 [rewrite]: %v", rr.SubQuestions)
	}
}

func TestParseRewrite_MissingRewriteReturnsNil(t *testing.T) {
	for _, raw := range []string{`{}`, `{"rewrite":""}`, "不是JSON", ""} {
		if rr := parseRewrite(raw); rr != nil {
			t.Errorf("非法输入 %q 应返回 nil: %+v", raw, rr)
		}
	}
}

// ========== RewriteWithSplit 端到端 ==========

type fakeChat struct {
	resp    string
	err     error
	lastReq llm.ChatRequest
	called  bool
}

func (f *fakeChat) Chat(_ context.Context, req llm.ChatRequest) (string, error) {
	f.called = true
	f.lastReq = req
	return f.resp, f.err
}

func rewriterFixture(chat chatClient, enabled bool) *Rewriter {
	loader := NewMappingLoader(nil, nil)
	loader.mappingsOverride = []TermMappingDO{
		{SourceTerm: "保司", TargetTerm: "保险公司", MatchType: 1, Enabled: 1, Priority: 100},
	}
	return NewRewriter(loader, chat, nil, enabled)
}

func TestRewriteWithSplit_NormalizesBeforeLLM(t *testing.T) {
	fake := &fakeChat{resp: `{"rewrite":"保险公司的理赔流程","sub_questions":["保险公司的理赔流程"]}`}
	r := rewriterFixture(fake, true)

	rr := r.RewriteWithSplit(context.Background(), "保司的理赔流程是什么", nil)

	if !fake.called {
		t.Fatal("应调用 LLM")
	}
	userMsg := fake.lastReq.Messages[len(fake.lastReq.Messages)-1]
	if userMsg.Role != "user" || !strings.Contains(userMsg.Content, "保险公司") {
		t.Errorf("LLM 输入应为归一化后的问题: %+v", userMsg)
	}
	if rr.Rewritten != "保险公司的理赔流程" {
		t.Errorf("改写结果错误: %+v", rr)
	}
}

func TestRewriteWithSplit_HistoryCappedAtFour(t *testing.T) {
	fake := &fakeChat{resp: `{"rewrite":"q","sub_questions":["q"]}`}
	r := rewriterFixture(fake, true)
	history := []llm.Message{
		{Role: "user", Content: "h1"}, {Role: "assistant", Content: "h2"},
		{Role: "user", Content: "h3"}, {Role: "assistant", Content: "h4"},
		{Role: "user", Content: "h5"}, {Role: "assistant", Content: "h6"},
	}
	r.RewriteWithSplit(context.Background(), "问题", history)

	// system + 最近4条历史 + user = 6
	if len(fake.lastReq.Messages) != 6 {
		t.Fatalf("消息数应为 6(system+4history+user)，实际 %d", len(fake.lastReq.Messages))
	}
	if fake.lastReq.Messages[1].Content != "h3" {
		t.Errorf("应只保留最近 4 条历史，首条应为 h3: %q", fake.lastReq.Messages[1].Content)
	}
}

func TestRewriteWithSplit_LLMFailureFallsBack(t *testing.T) {
	fake := &fakeChat{err: fmt.Errorf("all models unavailable")}
	r := rewriterFixture(fake, true)

	rr := r.RewriteWithSplit(context.Background(), "保司理赔？保司投诉？", nil)

	if rr.Rewritten != "保险公司理赔？保险公司投诉？" {
		t.Errorf("失败时 Rewritten 应为归一化原文: %q", rr.Rewritten)
	}
	if len(rr.SubQuestions) != 2 {
		t.Errorf("失败时应规则拆分: %v", rr.SubQuestions)
	}
}

func TestRewriteWithSplit_DisabledSkipsLLM(t *testing.T) {
	fake := &fakeChat{resp: `{"rewrite":"x"}`}
	r := rewriterFixture(fake, false)

	rr := r.RewriteWithSplit(context.Background(), "保司理赔", nil)

	if fake.called {
		t.Error("改写关闭时不应调用 LLM")
	}
	if rr.Rewritten != "保险公司理赔" {
		t.Errorf("关闭时仍应做同义词归一化: %q", rr.Rewritten)
	}
}
