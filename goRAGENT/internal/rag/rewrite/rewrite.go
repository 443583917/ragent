package rewrite

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/prompt"
	"go.uber.org/zap"
)

// rewritePromptPath 改写模板（和 Java QUERY_REWRITE_AND_SPLIT_PROMPT_PATH 对应）
const rewritePromptPath = "user-question-rewrite.st"

// historyKeepForRewrite 改写时最多携带的历史消息条数（和 Java buildRewriteRequest 一致）
const historyKeepForRewrite = 4

// RewriteResult 改写结果
type RewriteResult struct {
	Rewritten    string
	SubQuestions []string
}

// chatClient LLM 同步调用抽象（*llm.ChatService 满足）
type chatClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

// Rewriter 查询改写器：同义词归一化 → LLM 改写+子问题拆分 → 失败回退规则拆分
// （和 Java MultiQuestionRewriteService 对应）
type Rewriter struct {
	loader  *MappingLoader
	llm     chatClient
	prompts *prompt.TemplateLoader
	enabled bool
}

func NewRewriter(loader *MappingLoader, llmSvc chatClient, prompts *prompt.TemplateLoader, enabled bool) *Rewriter {
	if prompts == nil {
		prompts = prompt.NewTemplateLoader()
	}
	return &Rewriter{loader: loader, llm: llmSvc, prompts: prompts, enabled: enabled}
}

// RewriteWithSplit 改写并拆分子问题。任何失败降级为「归一化原文 + 规则拆分」，不阻断问答。
func (r *Rewriter) RewriteWithSplit(ctx context.Context, question string, history []llm.Message) RewriteResult {
	normalized := r.loader.Normalize(question)
	fallback := RewriteResult{Rewritten: normalized, SubQuestions: ruleBasedSplit(normalized)}
	if !r.enabled {
		return fallback
	}

	sysPrompt, err := r.prompts.Load(rewritePromptPath)
	if err != nil {
		zap.L().Error("加载改写模板失败", zap.Error(err))
		return fallback
	}

	messages := []llm.Message{{Role: "system", Content: sysPrompt}}
	messages = append(messages, recentHistory(history, historyKeepForRewrite)...)
	messages = append(messages, llm.Message{Role: "user", Content: normalized})

	temp, topP := 0.1, 0.3
	raw, err := r.llm.Chat(ctx, llm.ChatRequest{Messages: messages, Temperature: &temp, TopP: &topP})
	if err != nil {
		zap.L().Warn("查询改写 LLM 调用失败，回退规则拆分", zap.Error(err))
		return fallback
	}
	rr := parseRewrite(raw)
	if rr == nil {
		zap.L().Warn("查询改写输出解析失败，回退规则拆分", zap.String("raw", truncate(raw, 200)))
		return fallback
	}
	zap.L().Info("查询改写完成",
		zap.String("rewritten", rr.Rewritten), zap.Int("subQuestions", len(rr.SubQuestions)))
	return *rr
}

// recentHistory 只保留最近 n 条 user/assistant 消息
func recentHistory(history []llm.Message, n int) []llm.Message {
	var filtered []llm.Message
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) > n {
		filtered = filtered[len(filtered)-n:]
	}
	return filtered
}

// parseRewrite 解析 LLM 输出 {"rewrite","should_split","sub_questions"}；
// rewrite 缺失返回 nil，sub_questions 缺省为 [rewrite]
func parseRewrite(raw string) *RewriteResult {
	raw = stripMarkdownCodeFence(raw)
	if raw == "" {
		return nil
	}
	var out struct {
		Rewrite      string   `json:"rewrite"`
		SubQuestions []string `json:"sub_questions"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	out.Rewrite = strings.TrimSpace(out.Rewrite)
	if out.Rewrite == "" {
		return nil
	}
	var subs []string
	for _, s := range out.SubQuestions {
		if s = strings.TrimSpace(s); s != "" {
			subs = append(subs, s)
		}
	}
	if len(subs) == 0 {
		subs = []string{out.Rewrite}
	}
	return &RewriteResult{Rewritten: out.Rewrite, SubQuestions: subs}
}

var splitDelimiters = regexp.MustCompile(`[?？。；;\n]+`)

// ruleBasedSplit 规则拆分 fallback：按 ?？。；;\n 切分，每段补问号（和 Java ruleBasedSplit 一致）
func ruleBasedSplit(text string) []string {
	parts := splitDelimiters.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasSuffix(p, "?") && !strings.HasSuffix(p, "？") {
			p += "？"
		}
		result = append(result, p)
	}
	if len(result) == 0 {
		result = []string{text}
	}
	return result
}

// stripMarkdownCodeFence 剥离 ```json ... ``` 包裹
func stripMarkdownCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	} else {
		return ""
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
