package mcp

import (
	"context"
	"strings"
	"sync"

	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

// Executor MCP 工具执行编排器
type Executor struct {
	registry  *Registry
	extractor *Extractor
	formatter *Formatter
}

func NewExecutor(registry *Registry, extractor *Extractor, formatter *Formatter) *Executor {
	return &Executor{registry: registry, extractor: extractor, formatter: formatter}
}

// Execute 并行执行所有 MCP 意图对应的工具调用
func (e *Executor) Execute(ctx context.Context, subs []model.SubQuestionIntent, question string) []McpResult {
	type pendingCall struct {
		subQuestion string
		tool        *RegisteredTool
	}
	var calls []pendingCall

	for _, sub := range subs {
		for _, ns := range sub.NodeScores {
			if ns.Node == nil || !ns.Node.IsMCP {
				continue
			}
			tool := e.registry.GetByIntent(ns.Node.McpToolID)
			if tool == nil {
				zap.L().Warn("MCP 工具未注册", zap.String("mcp_tool_id", ns.Node.McpToolID))
				continue
			}
			sq := sub.SubQuestion
			if sq == "" {
				sq = question
			}
			calls = append(calls, pendingCall{subQuestion: sq, tool: tool})
		}
	}

	if len(calls) == 0 {
		return nil
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []McpResult
	)

	for _, call := range calls {
		wg.Add(1)
		go func(c pendingCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					zap.L().Error("MCP 执行 panic", zap.Any("recover", r))
				}
			}()

			params, err := e.extractor.Extract(ctx, c.subQuestion, c.tool, "")
			if err != nil {
				mu.Lock()
				results = append(results, McpResult{
					SubQuestion: c.subQuestion,
					ToolName:    c.tool.ToolName,
					Error:       "参数提取失败: " + err.Error(),
				})
				mu.Unlock()
				return
			}

			callResult, err := c.tool.Client.CallTool(ctx, c.tool.ToolName, params)
			if err != nil {
				mu.Lock()
				results = append(results, McpResult{
					SubQuestion: c.subQuestion,
					ToolName:    c.tool.ToolName,
					Error:       "工具调用失败: " + err.Error(),
				})
				mu.Unlock()
				return
			}

			var contentParts []string
			for _, item := range callResult.Content {
				if item.Type == "text" && item.Text != "" {
					contentParts = append(contentParts, item.Text)
				}
			}

			mu.Lock()
			results = append(results, McpResult{
				SubQuestion: c.subQuestion,
				ToolName:    c.tool.ToolName,
				Content:     strings.Join(contentParts, "\n"),
			})
			mu.Unlock()
		}(call)
	}
	wg.Wait()

	zap.L().Info("MCP 执行完成", zap.Int("calls", len(calls)), zap.Int("results", len(results)))
	return results
}

// HasMCPIntent 检查是否有 MCP 意图
func HasMCPIntent(subs []model.SubQuestionIntent) bool {
	for _, sub := range subs {
		for _, ns := range sub.NodeScores {
			if ns.Node != nil && ns.Node.IsMCP {
				return true
			}
		}
	}
	return false
}

// HasKBIntent 检查是否有 KB 意图
func HasKBIntent(subs []model.SubQuestionIntent) bool {
	for _, sub := range subs {
		for _, ns := range sub.NodeScores {
			if ns.Node != nil && ns.Node.IsKB {
				return true
			}
		}
	}
	return false
}
