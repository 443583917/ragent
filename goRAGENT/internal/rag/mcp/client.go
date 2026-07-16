package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ToolDef MCP 工具定义
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// CallResult tools/call 返回
type CallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

// ContentItem text/resource/blob
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// InitResult initialize 返回
type InitResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Capabilities map[string]any `json:"capabilities"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client MCP HTTP JSON-RPC 客户端
type Client struct {
	serverURL string
	http      *http.Client
}

func NewClient(serverURL string, timeout time.Duration) *Client {
	return &Client{
		serverURL: serverURL,
		http:      &http.Client{Timeout: timeout},
	}
}

func (c *Client) call(ctx context.Context, method string, params any) (*rpcResponse, error) {
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})

	url := c.serverURL
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MCP RPC 失败: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var r rpcResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("MCP 解析响应失败: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", r.Error.Code, r.Error.Message)
	}
	return &r, nil
}

// Initialize 握手获取能力
func (c *Client) Initialize(ctx context.Context) (*InitResult, error) {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "goRAGENT", "version": "1.0.0"},
	}
	r, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}
	var result InitResult
	json.Unmarshal(r.Result, &result)

	// 发送 initialized 通知
	c.call(ctx, "notifications/initialized", nil)

	return &result, nil
}

// ListTools 发现工具列表
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	r, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(r.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/list 失败: %w", err)
	}
	return result.Tools, nil
}

// CallTool 调用工具
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	params := map[string]any{"name": name, "arguments": args}
	r, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var result CallResult
	if err := json.Unmarshal(r.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/call 失败: %w", err)
	}
	zap.L().Info("MCP 工具调用完成",
		zap.String("tool", name),
		zap.Int("content_items", len(result.Content)),
	)
	return &result, nil
}
