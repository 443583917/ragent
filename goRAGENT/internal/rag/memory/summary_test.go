package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"goRAGENT/internal/config"
	"goRAGENT/internal/infra/llm"
)

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

// ========== 摘要窗口纯逻辑 ==========

func TestSummaryCutoffIndex_HalfWindow(t *testing.T) {
	// (n-1)/2：8 条 → 下标 3；5 条 → 下标 2；1 条 → 下标 0
	for n, want := range map[int]int{8: 3, 5: 2, 1: 0, 2: 0} {
		if got := summaryCutoffIndex(n); got != want {
			t.Errorf("n=%d 应为 %d 实际 %d", n, want, got)
		}
	}
}

func TestShouldSkipSummary_CoveredWindow(t *testing.T) {
	if !shouldSkipSummary(100, 90) {
		t.Error("afterID≥historyStartID 应跳过（摘要已覆盖窗口）")
	}
	if shouldSkipSummary(80, 90) {
		t.Error("afterID<historyStartID 不应跳过")
	}
	if shouldSkipSummary(0, 90) {
		t.Error("无历史摘要(afterID=0)不应跳过")
	}
}

// ========== 摘要消息组装 ==========

func TestBuildSummaryMessages_Order(t *testing.T) {
	msgs := []ConversationMessageDO{
		{Role: "user", Content: "问1"},
		{Role: "assistant", Content: "答1"},
	}
	got := buildSummaryMessages("系统提示", msgs, "旧摘要", 200)
	// system → 旧摘要(assistant) → 对话原文 → user 合并指令
	if got[0].Role != "system" || got[0].Content != "系统提示" {
		t.Errorf("首条应为 system: %+v", got[0])
	}
	if got[1].Role != "assistant" || !strings.Contains(got[1].Content, "旧摘要") ||
		!strings.Contains(got[1].Content, "历史摘要") {
		t.Errorf("第二条应为历史摘要注入: %+v", got[1])
	}
	if got[2].Content != "问1" || got[3].Content != "答1" {
		t.Errorf("对话原文顺序错误: %+v", got[2:4])
	}
	last := got[len(got)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "200") || !strings.Contains(last.Content, "合并") {
		t.Errorf("末条应为合并指令: %+v", last)
	}
}

func TestBuildSummaryMessages_NoExisting(t *testing.T) {
	got := buildSummaryMessages("sys", []ConversationMessageDO{{Role: "user", Content: "q"}}, "", 300)
	if len(got) != 3 { // system + 1 对话 + user 指令
		t.Fatalf("无旧摘要应为 3 条: %+v", got)
	}
}

// ========== 摘要包装 ==========

func TestDecorateSummary_WrapsWithTag(t *testing.T) {
	m := NewConversationMemory(&config.Config{}, nil, nil, nil, nil)
	got := m.decorateSummary("这是摘要内容")
	if !strings.Contains(got, "<conversation-summary>") || !strings.Contains(got, "这是摘要内容") {
		t.Errorf("应以 conversation-summary 标签包裹: %q", got)
	}
}

// ========== 会话标题 ==========

func titleFixture(chat chatClient) *ConversationMemory {
	cfg := &config.Config{Memory: config.MemoryConfig{TitleMaxLength: 10}}
	return NewConversationMemory(cfg, nil, nil, chat, nil)
}

func TestGenerateTitle_UsesLLM(t *testing.T) {
	fake := &fakeChat{resp: "OA系统使用咨询"}
	m := titleFixture(fake)
	got := m.generateTitle(context.Background(), "请问OA系统怎么用啊谢谢")
	if got != "OA系统使用咨询" {
		t.Errorf("应使用 LLM 标题: %q", got)
	}
	if !fake.called {
		t.Fatal("应调用 LLM")
	}
	user := fake.lastReq.Messages[0]
	if user.Role != "user" || !strings.Contains(user.Content, "请问OA系统怎么用啊谢谢") {
		t.Errorf("prompt 应含问题: %+v", user)
	}
	if strings.Contains(user.Content, "{question}") || strings.Contains(user.Content, "{title_max_chars}") {
		t.Errorf("占位符未替换: %s", user.Content)
	}
}

func TestGenerateTitle_FallbackOnError(t *testing.T) {
	m := titleFixture(&fakeChat{err: fmt.Errorf("unavailable")})
	got := m.generateTitle(context.Background(), "这是一个非常长的问题超过十个字了")
	if got != "这是一个非常长的问题" { // 截断 10 rune
		t.Errorf("失败应回退截断标题: %q", got)
	}
}

func TestGenerateTitle_CleansQuotesAndTruncates(t *testing.T) {
	m := titleFixture(&fakeChat{resp: "\"超长标题超长标题超长标题超长标题\"\n"})
	got := m.generateTitle(context.Background(), "问题")
	if strings.Contains(got, "\"") || len([]rune(got)) > 10 {
		t.Errorf("应去引号并按上限截断: %q", got)
	}
}
