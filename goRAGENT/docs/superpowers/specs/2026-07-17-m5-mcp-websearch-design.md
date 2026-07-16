# M5 MCP 工具执行 + You.com 联网检索 设计文档

> 日期: 2026-07-17 | 状态: draft | 关联: development-tasks.md M5 章节

## 一、目标

实装 MCP（Model Context Protocol）全套工具执行链路 + You.com 联网检索通道。完成后，意图命中 MCP 节点时自动调远程 MCP Server 获取业务数据；KB 检索无结果时兜底联网搜索。

## 二、现有基础

- IntentNode MCP 字段（`mcp_tool_id`/`param_prompt_template`/`Kind=2`）已完整
- `NodeRef.IsMCP` 已在 resolver 中派生
- Pipeline `isSystemOnly()` 已排除 MCP 意图，`singleIntentTemplate()` 支持节点模板
- MCP prompt 模板全有：`answer-chat-mcp.st`/`answer-chat-mcp-kb-mixed.st`/`mcp-parameter-extract.st`/`mcp-parameter-extract-user.st`
- `RetrievalContext.McpContext` 字段已声明（未使用）
- 检索通道体系完善（SearchChannel 接口 + MultiChannelEngine）

## 三、MCP 客户端 (`internal/rag/mcp/`)

### 3.1 协议

自研 HTTP JSON-RPC 客户端，对标 mineru/embedding 风格，零外部依赖。MCP 协议基于 HTTP+SSE 传输，JSON-RPC 2.0 格式：

```
POST {server_url}/message?sessionId=xxx
{"jsonrpc":"2.0","method":"tools/list","id":1}
```

核心 struct：
```go
type Client struct {
    serverURL string
    http      *http.Client
}
```

### 3.2 方法

| 方法 | MCP Method | 说明 |
|------|------|------|
| `Initialize(ctx) (*InitResult, error)` | `initialize` | 握手，获取 sessionId + capabilities |
| `ListTools(ctx) ([]ToolDef, error)` | `tools/list` | 发现工具列表 |
| `CallTool(ctx, name, args) (*CallResult, error)` | `tools/call` | 调用工具，返回 content |

`ToolDef` 含 `Name/Description/InputSchema`（JSON Schema）。`CallResult.Content` 返回工具输出的 text/blob array。

### 3.3 连接管理

`Initialize` 返回 `sessionId`，后续请求带 `?sessionId=xxx`。初始化时设置 `ClientInfo{Name:"goRAGENT", Version:"1.0.0"}`。

## 四、工具注册表 (`internal/rag/mcp/registry.go`)

```go
type Registry struct {
    tools map[string]*RegisteredTool // key = intent_node.mcp_tool_id → tool
}

type RegisteredTool struct {
    ToolID   string   // intent_node 中的 mcp_tool_id
    ToolName string   // MCP Server 返回的原始 tool name
    Client   *Client  // 指向所属 MCP Client
    ToolDef  ToolDef  // Name + Description + InputSchema
}
```

启动时注册流程：
1. 读取 `MCP_SERVERS` env → `[]McpServerConfig`
2. 逐 server：`NewClient(url) → Initialize() → ListTools()`
3. 每个 tool → `Registry.Register(mcpToolID, toolName, client, toolDef)`

查询：`Registry.GetByIntent(mcpToolID) → *RegisteredTool`

## 五、LLM 参数提取 (`internal/rag/mcp/extractor.go`)

```go
type Extractor struct {
    llm     *llm.ChatService
    prompts *prompt.TemplateLoader
}

// Extract 从用户问题中提取工具调用参数
// userQuestion: 用户原始问题
// tool: 命中的 MCP 工具（含 JSON Schema）
// customPrompt: intent_node.param_prompt_template（为空则用默认模板）
func (e *Extractor) Extract(ctx, userQuestion, toolDefJSON string, customPrompt string) (map[string]any, error)
```

流程：
1. System: `mcp-parameter-extract.st`（或 customPrompt 覆盖）
2. User: `mcp-parameter-extract-user.st`，占位符 `{tool_definition}` + `{user_question}`
3. LLM 调用（temp=0.1, topP=0.3），返回 JSON
4. JSON 容错解析（trim code fence、`""` → `{}`）

## 六、MCP 执行编排 (`internal/rag/mcp/executor.go`)

```go
type Executor struct {
    registry  *Registry
    extractor *Extractor
    formatter *Formatter
}

// McpResult 单次 MCP 工具调用结果
type McpResult struct {
    SubQuestion string
    ToolName    string
    Content     string
    Error       string
}

func (e *Executor) Execute(ctx, subs []retrieve.SubQuestionIntent, question string) []McpResult
```

执行流程：
1. 遍历 subs，过滤 IsMCP 意图
2. 每意图：`registry.GetByIntent(node.McpToolID)` → `extractor.Extract()` → `client.CallTool()`
3. `formatter.Format()` 将 tool result 转成 XML 文本 `<tool-data>...</tool-data>`
4. 结果收集入 `[]McpResult`（失败不阻断其他，记录 Error 字段）

## 七、Pipeline 改动 (`pipeline.go`)

### 7.1 新增接口

```go
type McpExecutor interface {
    Execute(ctx context.Context, subs []retrieve.SubQuestionIntent, question string) []mcp.McpResult
}
```

### 7.2 新增 MCP 执行步骤

在 `resolveIntents` 后、`guidance` 前插：

```go
// 3.5 MCP 工具执行（仅 MCP 意图）
var mcpResults []mcp.McpResult
if p.mcpExecutor != nil {
    mcpResults = p.mcpExecutor.Execute(ctx, subIntents, pipeCtx.Question)
}
```

### 7.3 场景分流

按 MCP/KB 意图组合分 4 场景：

| 场景 | 条件 | 行为 |
|------|------|------|
| KB_ONLY | 无 MCP 意图 | 现有流程不变 |
| MCP_ONLY | 全部 MCP，无 KB | 跳过检索，MCP 结果喂 LLM，用 `answer-chat-mcp.st` |
| MIXED | 有 MCP 也有 KB | MCP+KB 并行（MCP 结果+检索结果合并），用 `answer-chat-mcp-kb-mixed.st` |
| EMPTY | 无 MCP 无 KB | 空检索短路（已有） |

MCP 结果格式化：
```xml
<tool-data>
<result index="1"><question>子问题1</question><data>工具结果...</data></result>
</tool-data>
```

## 八、You.com 联网检索通道 (`internal/rag/retrieve/channels.go`)

### 8.1 通道定义

```go
const ChannelWebSearch SearchChannelType = "WEB_SEARCH"

type YouComWebSearchChannel struct {
    cfg    config.WebSearchConfig
    client *http.Client
}

func (c *YouComWebSearchChannel) Name() string     { return "YouComWebSearch" }
func (c *YouComWebSearchChannel) Priority() int     { return 20 }  // 最低优先级
func (c *YouComWebSearchChannel) Type() SearchChannelType { return ChannelWebSearch }
```

### 8.2 API 调用

```
GET https://api.ydc-index.io/search?query={query}&count={count}
Authorization: Bearer {api_key}
```

返回结构：`{hits: [{title, url, snippets: []}]}` → 每个 snippet join 成 chunk，score=0.5（兜底）。

### 8.3 IsEnabled 逻辑

`enabled=true AND apiKey 非空`。失败静默降级空结果。

## 九、配置扩展

```go
// McpConfig 远程 MCP Server 列表
type McpConfig struct {
    Servers []McpServerConfig
}

type McpServerConfig struct {
    Name string `json:"name"` // server 标识
    URL  string `json:"url"`  // MCP Server HTTP endpoint
}

// WebSearchConfig You.com Web Search
type WebSearchConfig struct {
    Enabled        bool
    APIKey         string
    APIURL         string // 默认 https://api.ydc-index.io/search
    Count          int    // 返回 snippet 数，默认 5
    TimeoutSeconds int    // 默认 10
}
```

环境变量：
```bash
MCP_SERVERS=[{"name":"weather","url":"http://localhost:8080/mcp"}]
WEB_SEARCH_ENABLED=false
WEB_SEARCH_API_KEY=your_ydc_key
WEB_SEARCH_COUNT=5
WEB_SEARCH_TIMEOUT_SECONDS=10
```

## 十、main.go 接线

```go
// M5: MCP 客户端 + 注册表 + 参数提取 + 执行器
if cfg.Mcp.Servers != nil {
    mcpRegistry := mcp.NewRegistry(cfg.Mcp.Servers)
    mcpExtractor := mcp.NewExtractor(llmSvc, prompts)
    mcpFormatter := mcp.NewFormatter()
    mcpExecutor := mcp.NewExecutor(mcpRegistry, mcpExtractor, mcpFormatter)
    ragPipeline.SetMcpExecutor(mcpExecutor)
}

// M5: You.com WebSearch 通道（最低优先级兜底）
if cfg.RAG.WebSearch.Enabled && cfg.RAG.WebSearch.APIKey != "" {
    webChannel := retrieve.NewYouComWebSearchChannel(cfg.RAG.WebSearch)
    searchChannels = append(searchChannels, webChannel)
}
```

## 十一、文件清单

| 文件 | 操作 | 说明 |
|------|:--:|------|
| `internal/rag/mcp/client.go` | 新 | MCP HTTP JSON-RPC 客户端 |
| `internal/rag/mcp/registry.go` | 新 | 工具注册表，启动时发现 |
| `internal/rag/mcp/extractor.go` | 新 | LLM 参数提取 |
| `internal/rag/mcp/formatter.go` | 新 | 工具结果→XML 格式化 |
| `internal/rag/mcp/executor.go` | 新 | MCP 执行编排 |
| `internal/rag/pipeline/pipeline.go` | 改 | 加 MCP 步骤 + 场景分流 |
| `internal/rag/retrieve/channels.go` | 改 | 加 YouComWebSearchChannel |
| `internal/rag/retrieve/channel.go` | 改 | 加 ChannelWebSearch 常量 |
| `internal/config/config.go` | 改 | 加 McpConfig + WebSearchConfig |
| `cmd/server/main.go` | 改 | 装配 MCP + WebSearch 组件 |
