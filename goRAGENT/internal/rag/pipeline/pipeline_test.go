package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"goRAGENT/internal/config"
	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/memory"
	"goRAGENT/internal/rag/prompt"
	"goRAGENT/internal/rag/retrieve"
	"goRAGENT/internal/rag/rewrite"
)

// ========== fakes ==========

// fakeMemory 记录调用的记忆服务
type fakeMemory struct {
	history          []memory.ChatMessage
	appendedUser     string
	assistantContent string
	assistantCID     string
	done             chan struct{}
}

func (f *fakeMemory) LoadAndAppend(_ context.Context, cid, uid string, msg memory.ChatMessage) []memory.ChatMessage {
	f.appendedUser = msg.Content
	return f.history
}

func (f *fakeMemory) AppendAssistant(_ context.Context, cid, uid, content string) string {
	f.assistantCID = cid
	f.assistantContent = content
	close(f.done)
	return "9527"
}

// fakeResolver 记录调用并返回预设意图
type fakeResolver struct {
	intents      []retrieve.SubQuestionIntent
	called       bool
	subQuestions []string
}

func (f *fakeResolver) ResolveAll(_ context.Context, qs []string) []retrieve.SubQuestionIntent {
	f.called = true
	f.subQuestions = qs
	return f.intents
}

// fakeRewriter 返回预设改写结果
type fakeRewriter struct {
	result   rewrite.RewriteResult
	question string
	history  []llm.Message
}

func (f *fakeRewriter) RewriteWithSplit(_ context.Context, q string, history []llm.Message) rewrite.RewriteResult {
	f.question = q
	f.history = history
	return f.result
}

// recordingChannel 记录收到的 SearchContext，可返回预设 chunks
type recordingChannel struct {
	got    *retrieve.SearchContext
	chunks []retrieve.RetrievedChunk
}

func (r *recordingChannel) Name() string                                             { return "recording" }
func (r *recordingChannel) Priority() int                                            { return 99 }
func (r *recordingChannel) Type() retrieve.SearchChannelType                         { return "RECORDING" }
func (r *recordingChannel) IsEnabled(context.Context, *retrieve.SearchContext) bool { return true }
func (r *recordingChannel) Search(_ context.Context, sc *retrieve.SearchContext) (*retrieve.ChannelResult, error) {
	r.got = sc
	return &retrieve.ChannelResult{ChannelType: "RECORDING", ChannelName: "recording", Chunks: r.chunks}, nil
}

// noopCallback 空回调
type noopCallback struct{}

func (noopCallback) OnContent(string)  {}
func (noopCallback) OnThinking(string) {}
func (noopCallback) OnComplete()       {}
func (noopCallback) OnError(error)     {}

// doneCallback 收集内容，OnComplete 时关闭 done
type doneCallback struct {
	mu        sync.Mutex
	content   strings.Builder
	done      chan struct{}
	messageID string
}

func (d *doneCallback) OnContent(chunk string) {
	d.mu.Lock()
	d.content.WriteString(chunk)
	d.mu.Unlock()
}
func (d *doneCallback) OnThinking(string)      {}
func (d *doneCallback) OnError(error)          {}
func (d *doneCallback) SetMessageID(id string) { d.messageID = id }
func (d *doneCallback) OnComplete()            { close(d.done) }

// capturingLLMServer 记录 LLM 请求消息并流式返回 tokens
type capturingLLMServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests [][]llm.Message
}

func newCapturingLLMServer(t *testing.T, tokens []string) *capturingLLMServer {
	t.Helper()
	c := &capturingLLMServer{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		c.mu.Lock()
		c.requests = append(c.requests, req.Messages)
		c.mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for _, tok := range tokens {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
			fl.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	return c
}

func (c *capturingLLMServer) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

// fakeGuidance 返回预设引导话术
type fakeGuidance struct {
	text   string
	called bool
}

func (f *fakeGuidance) Detect(_ context.Context, _ string, _ []retrieve.SubQuestionIntent) string {
	f.called = true
	return f.text
}

// ========== fixtures ==========

func llmConfig(baseURL string) *config.Config {
	return &config.Config{
		LLM: config.LLMConfig{Provider: "glm", GLMKey: "k", GLMBaseURL: baseURL, GLMModel: "m"},
		RAG: config.RAGConfig{TopK: 5},
	}
}

func buildPipeline(cfg *config.Config, mem MemoryService, rec *recordingChannel,
	rw QueryRewriter, resolver IntentResolver, gd GuidanceDetector) *SimplePipeline {
	prompts := prompt.NewTemplateLoader()
	var channels []retrieve.SearchChannel
	if rec != nil {
		channels = append(channels, rec)
	}
	engine := retrieve.NewRetrievalEngine(cfg.RAG, retrieve.NewMultiChannelEngine(channels, nil), prompts)
	if mem == nil {
		mem = memory.NewConversationMemory(cfg, nil, nil, nil, prompts)
	}
	return NewSimplePipeline(cfg, mem, llm.NewChatService(cfg), prompts, engine, rw, resolver, gd)
}

// ========== 测试 ==========

func TestExecute_PersistsAssistantOnComplete(t *testing.T) {
	llmSrv := fakeLLMServer(t, []string{"你好", "世界"})
	defer llmSrv.Close()

	fm := &fakeMemory{
		history: []memory.ChatMessage{{Role: "user", Content: "上轮问题"}, {Role: "assistant", Content: "上轮回答"}},
		done:    make(chan struct{}),
	}
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据文本"}}}
	p := buildPipeline(llmConfig(llmSrv.URL), fm, rec, nil, nil, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	pipeCtx := &Ctx{Question: "新问题", ConversationID: "conv-1", UserID: "u1"}
	if _, err := p.Execute(context.Background(), pipeCtx, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	select {
	case <-dcb.done:
	case <-time.After(5 * time.Second):
		t.Fatal("超时：流未完成")
	}
	select {
	case <-fm.done:
	case <-time.After(5 * time.Second):
		t.Fatal("超时：assistant 消息未落库")
	}
	if dcb.messageID != "9527" {
		t.Errorf("finish 前应回填消息 ID: %q", dcb.messageID)
	}
	if fm.assistantContent != "你好世界" {
		t.Errorf("落库内容应为完整回答: %q", fm.assistantContent)
	}
	if fm.assistantCID != "conv-1" {
		t.Errorf("会话 ID 错误: %q", fm.assistantCID)
	}
	if fm.appendedUser != "新问题" {
		t.Errorf("user 消息应经 LoadAndAppend 落库: %q", fm.appendedUser)
	}
	if len(pipeCtx.History) != 2 {
		t.Errorf("历史应加载 2 条: %+v", pipeCtx.History)
	}
}

func TestExecute_RewriteFlowsIntoResolverAndSearch(t *testing.T) {
	llmSrv := fakeLLMServer(t, []string{"答"})
	defer llmSrv.Close()

	fw := &fakeRewriter{result: rewrite.RewriteResult{
		Rewritten:    "改写后的查询",
		SubQuestions: []string{"子问题1", "子问题2"},
	}}
	fr := &fakeResolver{}
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据"}}}
	fm := &fakeMemory{
		history: []memory.ChatMessage{{Role: "user", Content: "h1"}},
		done:    make(chan struct{}),
	}
	p := buildPipeline(llmConfig(llmSrv.URL), fm, rec, fw, fr, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "原始问题", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	<-dcb.done

	if fw.question != "原始问题" {
		t.Errorf("改写器应收到原始问题: %q", fw.question)
	}
	if len(fw.history) != 1 || fw.history[0].Content != "h1" {
		t.Errorf("改写器应收到对话历史: %+v", fw.history)
	}
	if !fr.called || len(fr.subQuestions) != 2 || fr.subQuestions[0] != "子问题1" {
		t.Errorf("意图解析应收到拆分后的子问题: %+v", fr.subQuestions)
	}
	if rec.got == nil || rec.got.RewrittenQuestion != "改写后的查询" {
		t.Errorf("检索应使用改写后的查询: %+v", rec.got)
	}
}

func TestExecute_EmptyRetrievalShortCircuit(t *testing.T) {
	llmSrv := newCapturingLLMServer(t, []string{"不应出现"})
	defer llmSrv.srv.Close()

	fm := &fakeMemory{done: make(chan struct{})}
	rec := &recordingChannel{} // 无 chunks → 空检索
	p := buildPipeline(llmConfig(llmSrv.srv.URL), fm, rec, nil, nil, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "冷门问题", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	select {
	case <-dcb.done:
	case <-time.After(5 * time.Second):
		t.Fatal("超时：短路未完成")
	}
	if got := dcb.content.String(); got != "未检索到与问题相关的文档内容。" {
		t.Errorf("应返回空检索提示文案: %q", got)
	}
	if llmSrv.callCount() != 0 {
		t.Errorf("空检索短路不应调用 LLM，实际调用 %d 次", llmSrv.callCount())
	}
	select {
	case <-fm.done:
	case <-time.After(2 * time.Second):
		t.Fatal("提示文案也应落库为 assistant 消息")
	}
	if fm.assistantContent != "未检索到与问题相关的文档内容。" {
		t.Errorf("落库内容错误: %q", fm.assistantContent)
	}
}

func TestExecute_SingleIntentNodeTemplateOverridesSystemPrompt(t *testing.T) {
	llmSrv := newCapturingLLMServer(t, []string{"答"})
	defer llmSrv.srv.Close()

	fr := &fakeResolver{intents: []retrieve.SubQuestionIntent{{
		SubQuestion: "q",
		NodeScores: []retrieve.NodeScore{{
			Node:  &retrieve.NodeRef{ID: "hr", IsKB: true, PromptTemplate: "你是专属人事助手模板"},
			Score: 0.9,
		}},
	}}}
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据"}}}
	fm := &fakeMemory{done: make(chan struct{})}
	p := buildPipeline(llmConfig(llmSrv.srv.URL), fm, rec, nil, fr, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "请假", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	<-dcb.done

	if llmSrv.callCount() != 1 {
		t.Fatalf("应调用 LLM 1 次: %d", llmSrv.callCount())
	}
	sys := llmSrv.requests[0][0]
	if sys.Role != "system" || sys.Content != "你是专属人事助手模板" {
		t.Errorf("单意图节点模板应覆盖 system prompt: role=%s content=%q", sys.Role, sys.Content)
	}
}

func TestExecute_GuidanceShortCircuit(t *testing.T) {
	llmSrv := newCapturingLLMServer(t, []string{"不应出现"})
	defer llmSrv.srv.Close()

	fr := &fakeResolver{intents: []retrieve.SubQuestionIntent{{
		SubQuestion: "q",
		NodeScores: []retrieve.NodeScore{
			{Node: &retrieve.NodeRef{ID: "a", IsKB: true}, Score: 0.9},
			{Node: &retrieve.NodeRef{ID: "b", IsKB: true}, Score: 0.85},
		},
	}}}
	gd := &fakeGuidance{text: "请问你想了解哪个？1) A 2) B"}
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据"}}}
	fm := &fakeMemory{done: make(chan struct{})}
	p := buildPipeline(llmConfig(llmSrv.srv.URL), fm, rec, nil, fr, gd)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "数据安全规范", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	select {
	case <-dcb.done:
	case <-time.After(5 * time.Second):
		t.Fatal("超时")
	}
	if !gd.called {
		t.Fatal("应调用歧义检测")
	}
	if got := dcb.content.String(); got != "请问你想了解哪个？1) A 2) B" {
		t.Errorf("应推送引导话术: %q", got)
	}
	if rec.got != nil {
		t.Error("引导短路不应执行检索")
	}
	if llmSrv.callCount() != 0 {
		t.Error("引导短路不应调用回答 LLM")
	}
	<-fm.done // 引导话术应落库
	if !strings.Contains(fm.assistantContent, "请问你想了解哪个") {
		t.Errorf("引导话术应落库: %q", fm.assistantContent)
	}
}

func TestExecute_SystemOnlyShortCircuit(t *testing.T) {
	llmSrv := newCapturingLLMServer(t, []string{"你好，我是小助手"})
	defer llmSrv.srv.Close()

	fr := &fakeResolver{intents: []retrieve.SubQuestionIntent{{
		SubQuestion: "你是谁",
		NodeScores: []retrieve.NodeScore{
			{Node: &retrieve.NodeRef{ID: "sys-chat", IsKB: false, IsMCP: false}, Score: 0.9},
		},
	}}}
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据"}}}
	fm := &fakeMemory{
		history: []memory.ChatMessage{{Role: "user", Content: "h1"}},
		done:    make(chan struct{}),
	}
	p := buildPipeline(llmConfig(llmSrv.srv.URL), fm, rec, nil, fr, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "你是谁", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	select {
	case <-dcb.done:
	case <-time.After(5 * time.Second):
		t.Fatal("超时")
	}
	if rec.got != nil {
		t.Error("SYSTEM 短路不应执行检索")
	}
	if llmSrv.callCount() != 1 {
		t.Fatalf("应直答调用 LLM 1 次: %d", llmSrv.callCount())
	}
	msgs := llmSrv.requests[0]
	if msgs[0].Role != "system" || msgs[0].Content == "" {
		t.Errorf("首条应为 system prompt: %+v", msgs[0])
	}
	if strings.Contains(msgs[len(msgs)-1].Content, "<documents>") {
		t.Error("SYSTEM 直答不应携带检索证据")
	}
	if msgs[1].Content != "h1" {
		t.Errorf("应携带对话历史: %+v", msgs[1])
	}
	if msgs[len(msgs)-1].Content != "你是谁" {
		t.Errorf("末条应为原始问题: %+v", msgs[len(msgs)-1])
	}
	if got := dcb.content.String(); got != "你好，我是小助手" {
		t.Errorf("直答内容错误: %q", got)
	}
}

func TestExecute_SystemOnlyCustomTemplateOverride(t *testing.T) {
	llmSrv := newCapturingLLMServer(t, []string{"答"})
	defer llmSrv.srv.Close()

	fr := &fakeResolver{intents: []retrieve.SubQuestionIntent{{
		SubQuestion: "q",
		NodeScores: []retrieve.NodeScore{
			{Node: &retrieve.NodeRef{ID: "sys", PromptTemplate: "自定义系统人格"}, Score: 0.9},
		},
	}}}
	fm := &fakeMemory{done: make(chan struct{})}
	p := buildPipeline(llmConfig(llmSrv.srv.URL), fm, nil, nil, fr, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "q", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	<-dcb.done
	if llmSrv.requests[0][0].Content != "自定义系统人格" {
		t.Errorf("节点模板应覆盖 system prompt: %q", llmSrv.requests[0][0].Content)
	}
}

func TestExecute_NilOptionalDepsStillWorks(t *testing.T) {
	llmSrv := fakeLLMServer(t, []string{"答"})
	defer llmSrv.Close()

	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "证据"}}}
	p := buildPipeline(llmConfig(llmSrv.URL), nil, rec, nil, nil, nil)

	dcb := &doneCallback{done: make(chan struct{})}
	if _, err := p.Execute(context.Background(),
		&Ctx{Question: "问题", ConversationID: "c1", UserID: "u1"}, dcb); err != nil {
		t.Fatalf("nil rewriter/resolver 不应报错: %v", err)
	}
	<-dcb.done
	if rec.got == nil || rec.got.RewrittenQuestion != "问题" {
		t.Errorf("无改写器时应用原始问题检索: %+v", rec.got)
	}
	if len(rec.got.Intents) != 0 {
		t.Errorf("nil resolver 时 Intents 应为空: %+v", rec.got.Intents)
	}
}
