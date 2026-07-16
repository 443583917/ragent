package pipeline

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/framework/userctx"
	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/memory"
	"goRAGENT/internal/rag/prompt"
	"goRAGENT/internal/rag/retrieve"
)

// fakeLLMServer 模拟 OpenAI 兼容的流式 /chat/completions 端点，
// 逐 token 发送 SSE（带小延迟，模拟真实 LLM 出词节奏）。
func fakeLLMServer(t *testing.T, tokens []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for _, tok := range tokens {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
			fl.Flush()
			time.Sleep(30 * time.Millisecond)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
}

// newTestChatServer 用真实组件装配 ChatHandler（内存 stub / 空检索通道），
// 并启动真实 HTTP 服务器 —— 崩溃 bug 依赖 net/http 的请求生命周期，
// httptest.ResponseRecorder 无法复现。
func newTestChatServer(t *testing.T, llmBaseURL string) *httptest.Server {
	t.Helper()
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("snowflake init: %v", err)
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider: "glm",
			GLMKey:   "test-key",
			GLMBaseURL: llmBaseURL,
			GLMModel: "test-model",
		},
		RAG: config.RAGConfig{TopK: 5},
	}

	llmSvc := llm.NewChatService(cfg)
	prompts := prompt.NewTemplateLoader()
	memSvc := memory.NewConversationMemory(cfg, nil, nil, llmSvc, prompts)
	// 提供固定检索结果，避免触发空检索短路
	rec := &recordingChannel{chunks: []retrieve.RetrievedChunk{{ID: "c1", Text: "测试证据"}}}
	engine := retrieve.NewRetrievalEngine(cfg.RAG,
		retrieve.NewMultiChannelEngine([]retrieve.SearchChannel{rec}, nil), prompts)
	pl := NewSimplePipeline(cfg, memSvc, llmSvc, prompts, engine, nil, nil, nil)
	h := NewSimpleChatHandler(cfg, pl)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 注入登录用户（绕过 JWT 中间件）
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.Set(c.Request.Context(),
			&userctx.LoginUser{UserID: "10001", Username: "tester"}))
		c.Next()
	})
	r.GET("/chat", h.StreamChat)
	return httptest.NewServer(r)
}

// TestStreamChat_DeliversFullAnswer 验证：SSE 流式问答必须把 LLM 的完整回答
// 推送给客户端，并以 finish + [DONE] 事件收尾。
//
// 回归目标（P0 崩溃 bug）：handler 在流式输出完成前返回，导致
//  1) request context 被取消，LLM 流被立即掐断（回答内容丢失）；
//  2) 后台 goroutine 向已被 net/http 回收的 ResponseWriter 写入 → panic → 进程崩溃。
func TestStreamChat_DeliversFullAnswer(t *testing.T) {
	tokens := []string{"你好", "，", "我是", "goRAGENT", "。"}
	llmSrv := fakeLLMServer(t, tokens)
	defer llmSrv.Close()

	srv := newTestChatServer(t, llmSrv.URL)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/chat?question=hello")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type 应为 text/event-stream，实际: %q", ct)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	body := string(b)

	for _, tok := range tokens {
		if !strings.Contains(body, tok) {
			t.Errorf("响应缺少 token %q\n完整响应:\n%s", tok, body)
		}
	}
	if !strings.Contains(body, "event:finish") {
		t.Errorf("响应缺少 finish 事件\n完整响应:\n%s", body)
	}
	if !strings.Contains(body, "event:done") {
		t.Errorf("响应缺少 done 事件\n完整响应:\n%s", body)
	}
}
