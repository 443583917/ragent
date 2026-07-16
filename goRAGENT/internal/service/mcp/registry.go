package mcp

import (
	"context"
	"fmt"
	"time"

	"goRAGENT/internal/config"
	"go.uber.org/zap"
)

// RegisteredTool 已注册的工具（mcp_tool_id → tool 映射）
type RegisteredTool struct {
	ToolID   string
	ToolName string
	Client   *Client
	ToolDef  ToolDef
}

// Registry MCP 工具注册表
type Registry struct {
	tools map[string]*RegisteredTool
}

// NewRegistry 创建注册表并从所有 MCP Server 发现工具
func NewRegistry(servers []config.McpServerConfig) *Registry {
	r := &Registry{tools: make(map[string]*RegisteredTool)}

	for _, srv := range servers {
		zap.L().Info("连接 MCP Server", zap.String("name", srv.Name), zap.String("url", srv.URL))

		client := NewClient(srv.URL, 30*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		if _, err := client.Initialize(ctx); err != nil {
			zap.L().Error("MCP 初始化失败", zap.String("server", srv.Name), zap.Error(err))
			cancel()
			continue
		}

		tools, err := client.ListTools(ctx)
		cancel()
		if err != nil {
			zap.L().Error("MCP 发现工具失败", zap.String("server", srv.Name), zap.Error(err))
			continue
		}

		for _, tool := range tools {
			toolID := fmt.Sprintf("%s:%s", srv.Name, tool.Name)
			r.tools[toolID] = &RegisteredTool{
				ToolID:   toolID,
				ToolName: tool.Name,
				Client:   client,
				ToolDef:  tool,
			}
			zap.L().Info("注册 MCP 工具", zap.String("tool_id", toolID), zap.String("server", srv.Name))
		}
	}

	return r
}

// GetByIntent 按意图节点的 mcpToolID 获取注册工具
func (r *Registry) GetByIntent(mcpToolID string) *RegisteredTool {
	if r == nil {
		return nil
	}
	return r.tools[mcpToolID]
}
