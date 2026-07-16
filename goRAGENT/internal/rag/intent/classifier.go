package intent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/prompt"
	"go.uber.org/zap"
)

// classifierPromptPath 分类 Prompt 模板（和 Java INTENT_CLASSIFIER_PROMPT_PATH 对应）
const classifierPromptPath = "intent-classifier.st"

// NodeScore 意图节点评分
type NodeScore struct {
	Node   *IntentNode
	Score  float64
	Reason string
}

// chatClient LLM 同步调用抽象（*llm.ChatService 天然满足）
type chatClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

// Classifier LLM 意图分类器（和 Java DefaultIntentClassifier 对应）
type Classifier struct {
	loader  *TreeLoader
	llm     chatClient
	prompts *prompt.TemplateLoader

	treeOverride []*IntentNode // 测试注入用
}

func NewClassifier(loader *TreeLoader, llmSvc chatClient, prompts *prompt.TemplateLoader) *Classifier {
	if prompts == nil {
		prompts = prompt.NewTemplateLoader()
	}
	return &Classifier{loader: loader, llm: llmSvc, prompts: prompts}
}

// ClassifyTargets 对问题做意图分类，返回按 score 降序的候选意图。
// 任何失败（空树/LLM 失败/解析失败）都降级为空列表，不阻断问答。
func (c *Classifier) ClassifyTargets(ctx context.Context, question string) []NodeScore {
	roots := c.treeOverride
	if roots == nil {
		roots = c.loader.Load(ctx)
	}
	leaves := Leaves(roots)
	if len(leaves) == 0 {
		return nil
	}
	id2node := make(map[string]*IntentNode, len(leaves))
	for _, l := range leaves {
		id2node[l.ID] = l
	}

	tpl, err := c.prompts.Load(classifierPromptPath)
	if err != nil {
		zap.L().Error("加载意图分类模板失败", zap.Error(err))
		return nil
	}
	sysPrompt := strings.ReplaceAll(tpl, "{intent_list}", buildIntentList(leaves))

	temp, topP := 0.1, 0.3
	raw, err := c.llm.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: question},
		},
		Temperature: &temp,
		TopP:        &topP,
	})
	if err != nil {
		zap.L().Warn("意图分类 LLM 调用失败，降级为空意图", zap.Error(err))
		return nil
	}

	scores := parseResponse(raw, id2node)
	zap.L().Info("意图分类完成", zap.Int("candidates", len(leaves)), zap.Int("hits", len(scores)))
	return scores
}

// buildIntentList 叶子节点序列化为 {intent_list}（格式和 Java buildPrompt 一致）
func buildIntentList(leaves []*IntentNode) string {
	var b strings.Builder
	for i, n := range leaves {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("- id=" + n.ID)
		b.WriteString("\n  path=" + n.FullPath)
		if n.Description != "" {
			b.WriteString("\n  description=" + n.Description)
		}
		b.WriteString("\n  type=" + n.Kind.String())
		if n.Kind == KindMCP && n.McpToolID != "" {
			b.WriteString("\n  toolId=" + n.McpToolID)
		}
		if len(n.Examples) > 0 {
			b.WriteString("\n  examples=" + strings.Join(n.Examples, " / "))
		}
	}
	return b.String()
}

// parseResponse 解析 LLM 输出为 NodeScore 列表（按 score 降序）。
// 容错：剥离 markdown code fence、兼容 {"results":[...]} 包裹、
// 跳过缺 id/score 或 id 不在意图树中的项；解析失败返回空。
func parseResponse(raw string, id2node map[string]*IntentNode) []NodeScore {
	raw = stripMarkdownCodeFence(raw)
	if raw == "" {
		return nil
	}

	type item struct {
		ID     string   `json:"id"`
		Score  *float64 `json:"score"`
		Reason string   `json:"reason"`
	}
	var items []item
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		var wrapper struct {
			Results []item `json:"results"`
		}
		if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
			zap.L().Warn("意图分类输出解析失败", zap.String("raw", truncate(raw, 200)))
			return nil
		}
		items = wrapper.Results
	}

	var scores []NodeScore
	for _, it := range items {
		if it.ID == "" || it.Score == nil {
			continue
		}
		node, ok := id2node[it.ID]
		if !ok {
			zap.L().Warn("意图分类输出了未知 id，跳过", zap.String("id", it.ID))
			continue
		}
		scores = append(scores, NodeScore{Node: node, Score: *it.Score, Reason: it.Reason})
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].Score > scores[j].Score })
	return scores
}

// stripMarkdownCodeFence 剥离 ```json ... ``` 包裹（和 Java LLMResponseCleaner 对应）
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
