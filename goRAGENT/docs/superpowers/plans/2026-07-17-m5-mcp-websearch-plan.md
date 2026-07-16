# M5 MCP 工具执行 + You.com 联网检索 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 实装 MCP 远程工具执行（JSON-RPC 客户端 → 注册表 → LLM 参数提取 → 并行执行 → 结果格式化） + You.com 联网检索兜底通道。

**Architecture:** 新包 `internal/rag/mcp/` 含 5 文件（client/registry/extractor/formatter/executor），Pipeline 在 resolveIntents 后插入 MCP 执行步骤并按场景分流（KB_ONLY/MCP_ONLY/MIXED），WebSearch 作为 priority=20 的新 SearchChannel。

**Spec:** `docs/superpowers/specs/2026-07-17-m5-mcp-websearch-design.md`

## Global Constraints

- MCP 协议：HTTP JSON-RPC 2.0 自研，零外部 SDK 依赖（和 mineru/embedding 风格一致）
- MCP Server 列表：`MCP_SERVERS` JSON env var，`[]McpServerConfig`
- LLM 参数提取用 prompt 模板 `mcp-parameter-extract.st`（system）+ `mcp-parameter-extract-user.st`（user），temp=0.1
- You.com API：`GET https://api.ydc-index.io/search` + `Authorization: Bearer` header，priority=20
- 失败不中断主流程：MCP 工具调用失败回退 MIXED→KB_ONLY，WebSearch 失败静默降级空结果
- 新代码全部放在 `goRAGENT/` 目录下

---

### Task 1: Config 扩展 — McpConfig + WebSearchConfig

**Files:**
- Modify: `goRAGENT/internal/config/config.go`

**Interfaces:**
- Produces: `McpConfig` struct, `McpServerConfig` struct, `WebSearchConfig` struct, `Config.Mcp`, `Config.RAG.WebSearch`

- [ ] **Step 1: 加 WebSearchConfig 到 ChannelsConfig**

```go
// 在 ChannelsConfig 结构体中加字段
type ChannelsConfig struct {
	VectorGlobal  VectorGlobalConfig
	IntentDirected IntentDirectedConfig
	WebSearch     WebSearchConfig
}
```

- [ ] **Step 2: 加 WebSearchConfig struct + McpConfig struct**

在 config.go 末尾（`type MineruConfig` 之后）加：

```go
type WebSearchConfig struct {
	Enabled        bool
	APIKey         string
	APIURL         string
	Count          int
	TimeoutSeconds int
}

type McpConfig struct {
	Servers []McpServerConfig
}

type McpServerConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
```

- [ ] **Step 3: 在 Config 结构体中加 Mcp 字段**

```go
type Config struct {
	// ... existing fields ...
	Mcp McpConfig
}
```

- [ ] **Step 4: 在 Load() 中填充默认值**

在 Load() 函数的 `global = cfg` 之前追加：

```go
// MCP 服务器列表（JSON env var）
var mcpServers []McpServerConfig
if raw := os.Getenv("MCP_SERVERS"); raw != "" {
	json.Unmarshal([]byte(raw), &mcpServers)
}
```

在 Config 初始化中（和 Mineru 平级）：

```go
Mcp: McpConfig{
	Servers: mcpServers,
},
```

在 RAG.Search.Channels 中加：

```go
WebSearch: WebSearchConfig{
	Enabled:        envBool("WEB_SEARCH_ENABLED", false),
	APIKey:         envStr("WEB_SEARCH_API_KEY", ""),
	APIURL:         envStr("WEB_SEARCH_API_URL", "https://api.ydc-index.io/search"),
	Count:          envInt("WEB_SEARCH_COUNT", 5),
	TimeoutSeconds: envInt("WEB_SEARCH_TIMEOUT_SECONDS", 10),
},
```

config.go 需要 import `"encoding/json"`。

- [ ] **Step 5: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add goRAGENT/internal/config/config.go
git commit -m "feat(m5): add McpConfig + WebSearchConfig"
```

---

### Task 2: MCP 客户端 (`internal/rag/mcp/client.go`)

**Files:**
- Create: `goRAGENT/internal/rag/mcp/client.go`

**Interfaces:**
- Produces: `Client` struct, `ToolDef`, `CallResult`, `InitResult`, `NewClient(url, timeout)`, `Initialize()`, `ListTools()`, `CallTool()`

- [ ] **Step 1: 创建目录 + 写入 client.go**

```bash
mkdir -p goRAGENT/internal/rag/mcp
```

```go
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
	Type string `json:"type"` // "text" | "resource" | "blob"
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

// rpcRequest JSON-RPC 2.0 请求
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int    `json:"id"`
}

// rpcResponse JSON-RPC 2.0 响应
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
	sessionID string
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
	if c.sessionID != "" {
		url = c.serverURL + "?sessionId=" + c.sessionID
	}

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

// Initialize 握手获取 sessionId + 能力
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

	// 提取 sessionId（MCP SSE 实现通常在初始化响应头中返回，但也可能在 body 中）
	// 尝试从 response headers/meta 提取
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
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/mcp/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/mcp/client.go
git commit -m "feat(m5): add MCP HTTP JSON-RPC client (initialize/list/call)"
```

---

### Task 3: MCP 工具注册表 (`internal/rag/mcp/registry.go`)

**Files:**
- Create: `goRAGENT/internal/rag/mcp/registry.go`

**Interfaces:**
- Consumes: `Client`, `ToolDef` (Task 2), `McpServerConfig` (Task 1)
- Produces: `Registry` struct, `RegisteredTool` struct, `NewRegistry()`, `Register()`, `GetByIntent()`

- [ ] **Step 1: 写入 registry.go**

```go
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"go.uber.org/zap"
)

// RegisteredTool 已注册的工具（mcp_tool_id → tool 映射）
type RegisteredTool struct {
	ToolID  string
	ToolName string
	Client  *Client
	ToolDef ToolDef
}

// Registry MCP 工具注册表
type Registry struct {
	tools map[string]*RegisteredTool // key = intent_node.mcp_tool_id
}

// NewRegistry 创建注册表并从所有 MCP Server 发现工具
func NewRegistry(servers []config.McpServerConfig) *Registry {
	r := &Registry{tools: make(map[string]*RegisteredTool)}

	for _, srv := range servers {
		zap.L().Info("连接 MCP Server", zap.String("name", srv.Name), zap.String("url", srv.URL))

		client := NewClient(srv.URL, 30*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		if _, err := client.Initialize(ctx); err != nil {
			zap.L().Error("MCP 初始化失败",
				zap.String("server", srv.Name),
				zap.Error(err),
			)
			cancel()
			continue
		}

		tools, err := client.ListTools(ctx)
		cancel()
		if err != nil {
			zap.L().Error("MCP 发现工具失败",
				zap.String("server", srv.Name),
				zap.Error(err),
			)
			continue
		}

		for _, tool := range tools {
			// MCP tool 名即对应 intent_node.mcp_tool_id
			toolID := fmt.Sprintf("%s:%s", srv.Name, tool.Name)
			r.tools[toolID] = &RegisteredTool{
				ToolID:   toolID,
				ToolName: tool.Name,
				Client:   client,
				ToolDef:  tool,
			}
			zap.L().Info("注册 MCP 工具",
				zap.String("tool_id", toolID),
				zap.String("server", srv.Name),
			)
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
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/mcp/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/mcp/registry.go
git commit -m "feat(m5): add MCP tool registry (discover + register)"
```

---

### Task 4: MCP 结果格式化 (`internal/rag/mcp/formatter.go`)

**Files:**
- Create: `goRAGENT/internal/rag/mcp/formatter.go`

**Interfaces:**
- Consumes: `McpResult` (defined here), `CallResult` (Task 2)
- Produces: `Formatter` struct, `McpResult` struct, `Format()` method

- [ ] **Step 1: 写入 formatter.go**

```go
package mcp

import (
	"encoding/xml"
	"strings"
)

// McpResult MCP 工具调用结果
type McpResult struct {
	SubQuestion string
	ToolName    string
	Content     string
	Error       string
}

// Formatter 将 MCP 结果格式化为 LLM 上下文
type Formatter struct{}

func NewFormatter() *Formatter { return &Formatter{} }

// toolData XML 结构体（单问题）
type toolDataSingle struct {
	XMLName xml.Name  `xml:"tool-data"`
	Rules   string    `xml:"rules,omitempty"`
	Data    string    `xml:"data"`
}

// toolDataMulti XML 结构体（多子问题）
type toolDataMulti struct {
	XMLName xml.Name      `xml:"tool-data"`
	Results []toolResult  `xml:"result"`
}

type toolResult struct {
	Index     int    `xml:"index,attr"`
	Question  string `xml:"question"`
	Rules     string `xml:"rules,omitempty"`
	Data      string `xml:"data"`
}

// Format 将 MCP 结果列表格式化为 <tool-data> XML
func (f *Formatter) Format(results []McpResult) string {
	if len(results) == 0 {
		return ""
	}

	valid := make([]McpResult, 0, len(results))
	for _, r := range results {
		if r.Content != "" || r.Error != "" {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return ""
	}

	if len(valid) == 1 {
		r := valid[0]
		content := r.Content
		if r.Error != "" {
			content = "错误: " + r.Error
		}
		td := toolDataSingle{Data: content}
		b, _ := xml.MarshalIndent(td, "", "  ")
		return string(b)
	}

	// 多子问题
	var results []toolResult
	for i, r := range valid {
		content := r.Content
		if r.Error != "" {
			content = "错误: " + r.Error
		}
		results = append(results, toolResult{
			Index:    i + 1,
			Question: r.SubQuestion,
			Data:     content,
		})
	}
	td := toolDataMulti{Results: results}
	b, _ := xml.MarshalIndent(td, "", "  ")
	return string(b)
}

// FormatMixed 混合场景：MCP 数据 + KB 文档一起格式化到 context
// 返回 toolData XML 字符串（KB 文本由 pipeline 自行拼入 <documents>）
func (f *Formatter) FormatMixed(results []McpResult, kbText string) (toolData string, documents string) {
	toolData = f.Format(results)
	if kbText != "" {
		documents = "<documents>\n" + strings.TrimSpace(kbText) + "\n</documents>"
	}
	return
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/mcp/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/mcp/formatter.go
git commit -m "feat(m5): add MCP result formatter (XML <tool-data>)"
```

---

### Task 5: LLM 参数提取 (`internal/rag/mcp/extractor.go`)

**Files:**
- Create: `goRAGENT/internal/rag/mcp/extractor.go`

**Interfaces:**
- Consumes: `RegisteredTool` (Task 3), `llm.ChatService`, `prompt.TemplateLoader`
- Produces: `Extractor` struct, `NewExtractor()`, `Extract() map[string]any`

- [ ] **Step 1: 写入 extractor.go**

```go
package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/nageoffer/ragent/goRAGENT/internal/infra/llm"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/prompt"
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
	// 工具定义的 JSON Schema
	toolDefJSON, _ := json.Marshal(tool.ToolDef.InputSchema)

	// System prompt: 默认模板 或 节点自定义
	sysPrompt := customPrompt
	if strings.TrimSpace(sysPrompt) == "" {
		var err error
		sysPrompt, err = e.prompts.Load("mcp-parameter-extract.st")
		if err != nil {
			zap.L().Warn("加载 mcp-parameter-extract.st 失败", zap.Error(err))
			sysPrompt = "你是工具参数提取器，从用户问题中提取参数并以 JSON 格式输出。"
		}
	}

	// User prompt
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

	resp, err := e.llm.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// JSON 容错解析
	clean := strings.TrimSpace(resp.Content)
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
			zap.String("raw", resp.Content[:min(len(resp.Content), 200)]),
			zap.Error(err),
		)
		return map[string]any{}, nil
	}

	return params, nil
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/mcp/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/mcp/extractor.go
git commit -m "feat(m5): add LLM parameter extractor for MCP tools"
```

---

### Task 6: MCP 执行编排 (`internal/rag/mcp/executor.go`)

**Files:**
- Create: `goRAGENT/internal/rag/mcp/executor.go`

**Interfaces:**
- Consumes: `Registry` (Task 3), `Extractor` (Task 5), `Formatter` + `McpResult` (Task 4), `retrieve.SubQuestionIntent`
- Produces: `Executor` struct, `NewExecutor()`, `Execute() []McpResult`

- [ ] **Step 1: 写入 executor.go**

```go
package mcp

import (
	"context"
	"strings"
	"sync"

	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
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
func (e *Executor) Execute(ctx context.Context, subs []retrieve.SubQuestionIntent, question string) []McpResult {
	// 过滤 MCP 意图 + 收集待执行工具
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
			// 用子问题文本 或 原问题
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

	// 并行提取参数 + 调用工具
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

			// 1. LLM 参数提取
			params, err := e.extractor.Extract(ctx, c.subQuestion, c.tool, "")
			if err != nil {
				results = append(results, McpResult{
					SubQuestion: c.subQuestion,
					ToolName:    c.tool.ToolName,
					Error:       "参数提取失败: " + err.Error(),
				})
				return
			}

			// 2. 调用 MCP 工具
			callResult, err := c.tool.Client.CallTool(ctx, c.tool.ToolName, params)
			if err != nil {
				results = append(results, McpResult{
					SubQuestion: c.subQuestion,
					ToolName:    c.tool.ToolName,
					Error:       "工具调用失败: " + err.Error(),
				})
				return
			}

			// 3. 拼接 content
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

	zap.L().Info("MCP 执行完成",
		zap.Int("calls", len(calls)),
		zap.Int("results", len(results)),
	)
	return results
}

// HasMCPIntent 检查是否有 MCP 意图
func HasMCPIntent(subs []retrieve.SubQuestionIntent) bool {
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
func HasKBIntent(subs []retrieve.SubQuestionIntent) bool {
	for _, sub := range subs {
		for _, ns := range sub.NodeScores {
			if ns.Node != nil && ns.Node.IsKB {
				return true
			}
		}
	}
	return false
}

// HasAnyNonMCP 检查是否有非 MCP 意图（KB 或 SYSTEM）
func HasAnyNonMCP(subs []retrieve.SubQuestionIntent) bool {
	return HasKBIntent(subs)
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/mcp/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/mcp/executor.go
git commit -m "feat(m5): add MCP executor (parallel tool calls + parameter extraction)"
```

---

### Task 7: Pipeline 改动 — MCP 执行 + 场景分流

**Files:**
- Modify: `goRAGENT/internal/rag/pipeline/pipeline.go`

**Interfaces:**
- Consumes: `mcp.Executor`, `mcp.McpResult`, `mcp.HasMCPIntent`, `mcp.HasKBIntent` (Task 6), `mcp.Formatter` (Task 4)
- Produces: `SetMcpExecutor()` setter, MCP execution step, MCP_ONLY/MIXED scene routing

- [ ] **Step 1: 加 import + 接口**

在 `pipeline.go` 的 import 块中加：

```go
"github.com/nageoffer/ragent/goRAGENT/internal/rag/mcp"
```

在 `SimplePipeline` 结构体中加：

```go
type SimplePipeline struct {
	// ... existing fields ...
	mcpExecutor *mcp.Executor
	mcpFormatter *mcp.Formatter
}
```

加 setter：

```go
func (p *SimplePipeline) SetMcpExecutor(exec *mcp.Executor, formatter *mcp.Formatter) {
	p.mcpExecutor = exec
	p.mcpFormatter = formatter
}
```

- [ ] **Step 2: 在 `isSystemOnly` 后、retrieve 前插 MCP 执行步骤**

在 `Execute()` 方法中，step 4（guidance）和 step 5（systemOnly）之后，step 6（retrieve）之前插入：

```go
// 3.6 MCP 工具执行 + 场景分流
if p.mcpExecutor != nil && mcp.HasMCPIntent(subIntents) {
	mcpResults := p.mcpExecutor.Execute(ctx, subIntents, pipeCtx.Question)

	if !mcp.HasKBIntent(subIntents) {
		// MCP_ONLY：全部 MCP 意图，无 KB → 跳过检索，直接流式回答
		zap.L().Info("MCP_ONLY 场景", zap.Int("results", len(mcpResults)))
		toolData := p.mcpFormatter.Format(mcpResults)
		if toolData != "" {
			sysPrompt, _ := p.prompts.Load("answer-chat-mcp.st")
			sysPrompt = singleIntentTemplate(subIntents, sysPrompt)
			messages := []llm.Message{{Role: "system", Content: sysPrompt}}
			messages = append(messages, pipeCtx.History...)
			messages = append(messages, llm.Message{Role: "user", Content: toolData + "\n\n<question>" + pipeCtx.Question + "</question>"})
			temp := 0.0; topp := 1.0
			return p.llm.StreamChat(ctx, llm.ChatRequest{Messages: messages, Temperature: &temp, TopP: &topp}, wrapped), nil
		}
		// MCP 全部失败 → 回退 EMPTY 短路
		wrapped.OnContent(emptyRetrievalNotice)
		wrapped.OnComplete()
		return func() {}, nil
	}

	// MIXED：有 MCP 也有 KB → 存结果，后续和 KB 合并
	pipeCtx.McpResults = mcpResults
}
```

- [ ] **Step 3: 修改 `singleIntentTemplate` 使其接受默认回退**

将现有函数改为接受 fallback 参数，或新增重载：

```go
func singleIntentTemplate(subs []retrieve.SubQuestionIntent, fallback string) string {
	tmpl := singleIntentTemplateOnly(subs)
	if tmpl == "" {
		return fallback
	}
	return strings.TrimSpace(tmpl)
}

func singleIntentTemplateOnly(subs []retrieve.SubQuestionIntent) string { ... }
```

同时修改现有两处调用传入 fallback（原 `answer-chat-kb.st` 和 `answer-chat-system.st`）。

- [ ] **Step 4: 修改 retrieve 后的消息组装，支持 MIXED 场景**

在 `Ctx` 结构体中加 `McpResults` 字段：

```go
type Ctx struct {
	// ... existing ...
	McpResults []mcp.McpResult
}
```

在 retrieve 完成后的消息组装段（原来仅 KB），改为 MIXED 分支：

```go
// 6. 组装消息（MIXED 或 KB_ONLY）
if len(pipeCtx.McpResults) > 0 {
	// MIXED 场景：MCP + KB 合并
	toolData, docs := p.mcpFormatter.FormatMixed(pipeCtx.McpResults, kbText)
	sysPrompt, _ := p.prompts.Load("answer-chat-mcp-kb-mixed.st")
	sysPrompt = singleIntentTemplate(subIntents, sysPrompt)
	messages := []llm.Message{{Role: "system", Content: sysPrompt}}
	messages = append(messages, pipeCtx.History...)
	userContent := toolData
	if docs != "" {
		userContent += "\n\n" + docs
	}
	userContent += "\n\n<question>" + pipeCtx.Question + "</question>"
	messages = append(messages, llm.Message{Role: "user", Content: userContent})
	temp := 0.0; topp := 1.0
	return p.llm.StreamChat(ctx, llm.ChatRequest{Messages: messages, Temperature: &temp, TopP: &topp}, wrapped), nil
}
// KB_ONLY：现有逻辑不变 ...
```

同时对 `singleIntentTemplate` 调用全部传入 fallback（原默认模板）。

- [ ] **Step 5: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add goRAGENT/internal/rag/pipeline/pipeline.go
git commit -m "feat(m5): add MCP execution step + MCP_ONLY/MIXED scene routing in pipeline"
```

---

### Task 8: You.com WebSearch 通道

**Files:**
- Modify: `goRAGENT/internal/rag/retrieve/channel.go` — 加 `ChannelWebSearch` 常量
- Modify: `goRAGENT/internal/rag/retrieve/channels.go` — 加 `YouComWebSearchChannel`

**Interfaces:**
- Consumes: `WebSearchConfig` (Task 1)
- Produces: `ChannelWebSearch` 常量, `NewYouComWebSearchChannel()`, `YouComWebSearchChannel`

- [ ] **Step 1: channel.go 加常量**

```go
const (
	ChannelVectorGlobal   SearchChannelType = "VECTOR_GLOBAL"
	ChannelIntentDirected SearchChannelType = "INTENT_DIRECTED"
	ChannelKeyword        SearchChannelType = "KEYWORD"
	ChannelWebSearch      SearchChannelType = "WEB_SEARCH"
)
```

- [ ] **Step 2: channels.go 加 YouComWebSearchChannel**

在 channels.go 末尾追加：

```go
package retrieve

import (
	// ... add to imports:
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"go.uber.org/zap"
)

// YouComWebSearchChannel You.com 联网检索通道（priority=20, 最低级别兜底）
type YouComWebSearchChannel struct {
	cfg    config.WebSearchConfig
	client *http.Client
}

func NewYouComWebSearchChannel(cfg config.WebSearchConfig) *YouComWebSearchChannel {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	return &YouComWebSearchChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (c *YouComWebSearchChannel) Name() string             { return "YouComWebSearch" }
func (c *YouComWebSearchChannel) Priority() int              { return 20 }
func (c *YouComWebSearchChannel) Type() SearchChannelType    { return ChannelWebSearch }

func (c *YouComWebSearchChannel) IsEnabled(ctx context.Context, sc *SearchContext) bool {
	return c.cfg.Enabled && c.cfg.APIKey != ""
}

func (c *YouComWebSearchChannel) Search(ctx context.Context, sc *SearchContext) (*ChannelResult, error) {
	start := time.Now()

	query := sc.RewrittenQuestion
	if query == "" {
		query = sc.OriginalQuestion
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("count", fmt.Sprintf("%d", c.cfg.Count))

	apiURL := c.cfg.APIURL
	if apiURL == "" {
		apiURL = "https://api.ydc-index.io/search"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		zap.L().Warn("You.com 搜索失败", zap.Error(err))
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}
	defer resp.Body.Close()

	var result struct {
		Hits []struct {
			Title    string   `json:"title"`
			URL      string   `json:"url"`
			Snippets []string `json:"snippets"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		zap.L().Warn("You.com 解析失败", zap.Error(err))
		return emptyResult(ChannelWebSearch, c.Name(), start), nil
	}

	var chunks []RetrievedChunk
	for _, hit := range result.Hits {
		for _, snippet := range hit.Snippets {
			chunks = append(chunks, RetrievedChunk{
				ID:    hit.URL,
				Text:  snippet,
				Score: 0.5,
				Metadata: map[string]any{
					"title": hit.Title,
					"url":   hit.URL,
					"source": "web_search",
				},
			})
		}
	}

	latency := time.Since(start).Milliseconds()
	zap.L().Info("You.com 搜索完成",
		zap.Int("chunks", len(chunks)),
		zap.Int64("latency_ms", latency),
	)

	return &ChannelResult{
		ChannelType: ChannelWebSearch,
		ChannelName: c.Name(),
		Chunks:      chunks,
		LatencyMs:   latency,
	}, nil
}
```

- [ ] **Step 3: 验证编译**

```bash
cd goRAGENT && go build ./internal/rag/retrieve/...
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/internal/rag/retrieve/channel.go goRAGENT/internal/rag/retrieve/channels.go
git commit -m "feat(m5): add You.com WebSearch channel (priority 20)"
```

---

### Task 9: main.go 接线

**Files:**
- Modify: `goRAGENT/cmd/server/main.go`

**Interfaces:**
- Consumes: All M5 components (Tasks 1-8)
- Produces: Complete wiring

- [ ] **Step 1: 加 import**

```go
"github.com/nageoffer/ragent/goRAGENT/internal/rag/mcp"
```

- [ ] **Step 2: 在 main.go 中装配 MCP + WebSearch**

在 `ragPipeline` 创建之后（`chatHandler` 之前）加：

```go
// M5: MCP 工具执行
if len(cfg.Mcp.Servers) > 0 {
	mcpRegistry := mcp.NewRegistry(cfg.Mcp.Servers)
	mcpExtractor := mcp.NewExtractor(llmSvc, prompts)
	mcpFormatter := mcp.NewFormatter()
	mcpExecutor := mcp.NewExecutor(mcpRegistry, mcpExtractor, mcpFormatter)
	ragPipeline.SetMcpExecutor(mcpExecutor, mcpFormatter)
	zap.L().Info("MCP 已启用", zap.Int("servers", len(cfg.Mcp.Servers)))
}
```

- [ ] **Step 3: 加 WebSearch 通道**

在 `searchChannels` append 块的结尾（`mvStore` 检查内部）加：

```go
// M5: You.com 联网检索（最低优先级兜底）
if cfg.RAG.Search.Channels.WebSearch.Enabled && cfg.RAG.Search.Channels.WebSearch.APIKey != "" {
	searchChannels = append(searchChannels,
		retrieve.NewYouComWebSearchChannel(cfg.RAG.Search.Channels.WebSearch),
	)
}
```

- [ ] **Step 4: 验证编译**

```bash
cd goRAGENT && go build ./cmd/server/...
```

- [ ] **Step 5: Commit**

```bash
git add goRAGENT/cmd/server/main.go
git commit -m "feat(m5): wire MCP executor + You.com WebSearch in main.go"
```

---

### Task 10: 集成验证

- [ ] **Step 1: 编译 + 测试**

```bash
cd goRAGENT && go build ./... && go test ./... -count=1
```
Expected: 编译通过，所有已有测试绿

- [ ] **Step 2: MCP 端到端验证（可选，需 MCP Server）**

启动简单的 MCP test server 后：
```bash
export MCP_SERVERS='[{"name":"test","url":"http://localhost:8080/mcp"}]'
go run ./cmd/server/
# 创建 MCP 意图节点 → 提问 → 验证 MCP 工具被调用
```

- [ ] **Step 3: Commit 微调**

```bash
git add -A && git diff --cached --stat
git commit -m "chore(m5): integration verification tweaks"
```
