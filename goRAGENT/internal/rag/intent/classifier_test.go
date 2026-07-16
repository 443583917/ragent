package intent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"goRAGENT/internal/infra/llm"
)

// ========== buildIntentList 序列化 ==========

func TestBuildIntentList_KBNode(t *testing.T) {
	n := &IntentNode{
		ID: "group-hr", FullPath: "集团信息化 > 人事",
		Description: "招聘、入职、考勤等人力资源相关问题",
		Kind:        KindKB,
		Examples:    []string{"请假流程是怎样的？", "试用期多久转正？"},
	}
	got := buildIntentList([]*IntentNode{n})
	want := "- id=group-hr\n" +
		"  path=集团信息化 > 人事\n" +
		"  description=招聘、入职、考勤等人力资源相关问题\n" +
		"  type=KB\n" +
		"  examples=请假流程是怎样的？ / 试用期多久转正？"
	if got != want {
		t.Errorf("KB 节点序列化错误:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildIntentList_MCPNodeHasToolID(t *testing.T) {
	n := &IntentNode{
		ID: "sales-data", FullPath: "销售 > 数据统计",
		Description: "销售数据统计", Kind: KindMCP, McpToolID: "sales_query",
	}
	got := buildIntentList([]*IntentNode{n})
	if !strings.Contains(got, "type=MCP") || !strings.Contains(got, "toolId=sales_query") {
		t.Errorf("MCP 节点应包含 type=MCP 和 toolId:\n%s", got)
	}
	if strings.Contains(got, "examples=") {
		t.Errorf("无 examples 时不应输出 examples 行:\n%s", got)
	}
}

func TestBuildIntentList_MultipleNodesBlankLineSeparated(t *testing.T) {
	nodes := []*IntentNode{
		{ID: "a", FullPath: "A", Kind: KindKB},
		{ID: "b", FullPath: "B", Kind: KindKB},
	}
	got := buildIntentList(nodes)
	if !strings.Contains(got, "\n\n") {
		t.Errorf("多节点应以空行分隔:\n%s", got)
	}
}

// ========== parseResponse 容错解析 ==========

func id2NodeFixture() map[string]*IntentNode {
	return map[string]*IntentNode{
		"biz-oa-intro":    {ID: "biz-oa-intro"},
		"biz-oa-security": {ID: "biz-oa-security"},
	}
}

func TestParseResponse_PlainJSONSortedDesc(t *testing.T) {
	raw := `[{"id":"biz-oa-intro","score":0.7,"reason":"r1"},{"id":"biz-oa-security","score":0.9,"reason":"r2"}]`
	got := parseResponse(raw, id2NodeFixture())
	if len(got) != 2 {
		t.Fatalf("应解析出 2 条，实际 %d", len(got))
	}
	if got[0].Node.ID != "biz-oa-security" || got[0].Score != 0.9 {
		t.Errorf("应按 score 降序，首条: %+v", got[0])
	}
}

func TestParseResponse_StripsMarkdownFence(t *testing.T) {
	raw := "```json\n[{\"id\":\"biz-oa-intro\",\"score\":0.8}]\n```"
	got := parseResponse(raw, id2NodeFixture())
	if len(got) != 1 || got[0].Node.ID != "biz-oa-intro" {
		t.Fatalf("应剥离 code fence 后解析成功: %+v", got)
	}
}

func TestParseResponse_ResultsWrapper(t *testing.T) {
	raw := `{"results":[{"id":"biz-oa-intro","score":0.8}]}`
	got := parseResponse(raw, id2NodeFixture())
	if len(got) != 1 {
		t.Fatalf("应兼容 results 包裹格式: %+v", got)
	}
}

func TestParseResponse_SkipsUnknownAndInvalid(t *testing.T) {
	raw := `[
		{"id":"not-exist","score":0.9},
		{"id":"biz-oa-intro"},
		{"score":0.5},
		{"id":"biz-oa-security","score":0.6}
	]`
	got := parseResponse(raw, id2NodeFixture())
	if len(got) != 1 || got[0].Node.ID != "biz-oa-security" {
		t.Fatalf("应跳过未知 id 和缺字段项: %+v", got)
	}
}

func TestParseResponse_GarbageReturnsEmpty(t *testing.T) {
	for _, raw := range []string{"", "不是JSON", "{}", "[]"} {
		if got := parseResponse(raw, id2NodeFixture()); len(got) != 0 {
			t.Errorf("非法输入 %q 应返回空: %+v", raw, got)
		}
	}
}

// ========== ClassifyTargets 端到端（fake LLM） ==========

type fakeChat struct {
	resp     string
	err      error
	lastReq  llm.ChatRequest
	called   bool
}

func (f *fakeChat) Chat(_ context.Context, req llm.ChatRequest) (string, error) {
	f.called = true
	f.lastReq = req
	return f.resp, f.err
}

func classifierFixture(t *testing.T, chat chatClient) *Classifier {
	t.Helper()
	// loader 无 DB/Redis → 空树；测试用 treeOverride 注入
	c := NewClassifier(NewTreeLoader(nil, nil), chat, nil)
	c.treeOverride = []*IntentNode{
		{ID: "d", Name: "域", FullPath: "域", Children: []*IntentNode{
			{ID: "group-hr", Name: "人事", FullPath: "域 > 人事", Description: "人事问题", Kind: KindKB},
		}},
	}
	return c
}

func TestClassifyTargets_BuildsPromptAndParses(t *testing.T) {
	fake := &fakeChat{resp: `[{"id":"group-hr","score":0.92,"reason":"命中"}]`}
	c := classifierFixture(t, fake)

	got := c.ClassifyTargets(context.Background(), "请假流程是怎样的？")

	if len(got) != 1 || got[0].Node.ID != "group-hr" || got[0].Score != 0.92 {
		t.Fatalf("分类结果错误: %+v", got)
	}
	if !fake.called {
		t.Fatal("应调用 LLM")
	}
	// system prompt 含模板正文 + 叶子列表；user 是原始问题
	if len(fake.lastReq.Messages) != 2 {
		t.Fatalf("应有 system+user 两条消息: %d", len(fake.lastReq.Messages))
	}
	sys := fake.lastReq.Messages[0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "id=group-hr") {
		t.Errorf("system prompt 应含意图列表: %s", sys.Content[:min(200, len(sys.Content))])
	}
	if strings.Contains(sys.Content, "{intent_list}") {
		t.Errorf("占位符 {intent_list} 未被替换")
	}
	user := fake.lastReq.Messages[1]
	if user.Role != "user" || user.Content != "请假流程是怎样的？" {
		t.Errorf("user 消息应为原始问题: %+v", user)
	}
	// 低温高确定性采样
	if fake.lastReq.Temperature == nil || *fake.lastReq.Temperature != 0.1 {
		t.Errorf("temperature 应为 0.1")
	}
	if fake.lastReq.TopP == nil || *fake.lastReq.TopP != 0.3 {
		t.Errorf("topP 应为 0.3")
	}
}

func TestClassifyTargets_LLMErrorReturnsEmpty(t *testing.T) {
	fake := &fakeChat{err: fmt.Errorf("all models unavailable")}
	c := classifierFixture(t, fake)
	if got := c.ClassifyTargets(context.Background(), "问题"); len(got) != 0 {
		t.Fatalf("LLM 失败应降级为空: %+v", got)
	}
}

func TestClassifyTargets_EmptyTreeSkipsLLM(t *testing.T) {
	fake := &fakeChat{resp: "[]"}
	c := NewClassifier(NewTreeLoader(nil, nil), fake, nil)
	if got := c.ClassifyTargets(context.Background(), "问题"); len(got) != 0 {
		t.Fatalf("空树应返回空: %+v", got)
	}
	if fake.called {
		t.Error("空树不应调用 LLM")
	}
}
