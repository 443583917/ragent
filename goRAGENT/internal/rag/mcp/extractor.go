package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/prompt"
	"go.uber.org/zap"
)

// Extractor LLM 参数提取器（对齐 Java LLMMcpParameterExtractor）
type Extractor struct {
	llm     *llm.ChatService
	prompts *prompt.TemplateLoader
}

func NewExtractor(llmSvc *llm.ChatService, tmpl *prompt.TemplateLoader) *Extractor {
	return &Extractor{llm: llmSvc, prompts: tmpl}
}

// Extract 从用户问题中提取 MCP 工具调用参数
func (e *Extractor) Extract(ctx context.Context, userQuestion string, tool *RegisteredTool, customPrompt string) (map[string]any, error) {
	toolDefJSON, _ := json.Marshal(tool.ToolDef.InputSchema)

	sysPrompt := customPrompt
	if strings.TrimSpace(sysPrompt) == "" {
		var err error
		sysPrompt, err = e.prompts.Load("mcp-parameter-extract.st")
		if err != nil {
			zap.L().Warn("加载 mcp-parameter-extract.st 失败", zap.Error(err))
			sysPrompt = "你是工具参数提取器，从用户问题中提取参数并以 JSON 格式输出。"
		}
	}

	userPrompt, err := e.prompts.Load("mcp-parameter-extract-user.st")
	if err != nil {
		userPrompt = "用户问题: {user_question}\n\n工具定义: {tool_definition}\n\n请提取参数（JSON）："
	}
	userPrompt = strings.Replace(userPrompt, "{user_question}", userQuestion, 1)
	userPrompt = strings.Replace(userPrompt, "{tool_definition}", string(toolDefJSON), 1)

	temp := 0.1
	topp := 0.3
	req := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: &temp,
		TopP:        &topp,
	}

	content, err := e.llm.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	if clean == "" || clean == "\"\"" {
		return map[string]any{}, nil
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(clean), &params); err != nil {
		zap.L().Warn("MCP 参数提取 JSON 解析失败，使用空参数",
			zap.String("raw", clean[:minInt(len(clean), 200)]),
			zap.Error(err),
		)
		return map[string]any{}, nil
	}

	return params, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
