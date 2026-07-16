# goRAGENT 架构设计文档

> 版本: v1.0.0 | 日期: 2026-07-16 | 基于 tRPC-Agent-Go v1.10.0

## 一、模块架构

```
goRAGENT/
├── cmd/server/main.go          # 主服务（Gin HTTP + GraphAgent Pipeline）
├── cmd/mcp-server/main.go      # MCP Server（独立进程，端口 9099）
│
├── internal/
│   ├── framework/              # ≈ Java framework 模块（精简 50%+）
│   │   ├── sse/                SSE 封装（Emitter + 事件类型, 线程安全）
│   │   ├── jwt/                JWT 鉴权中间件
│   │   ├── userctx/            用户上下文（等价于 UserContext TTL）
│   │   ├── ratelimit/          分布式公平排队限流
│   │   ├── snowflake/          Snowflake 分布式 ID
│   │   └── response/           统一响应体 + 错误码
│   │
│   ├── infra/                  # ≈ Java infra-ai 模块
│   │   ├── llm/                Chat 客户端 + ModelRouter + CircuitBreaker
│   │   ├── embedding/          Embedding 客户端
│   │   ├── rerank/             Rerank 客户端（Cohere/百炼）
│   │   └── mineru/             MinerU PDF 解析 HTTP 客户端
│   │
│   ├── rag/                    # ≈ Java bootstrap/rag 包
│   │   ├── pipeline/           ★ GraphAgent StateGraph 定义
│   │   ├── intent/             意图节点加载 + LLM 分类 + 解析
│   │   ├── rewrite/            查询改写 + 同义词归一化
│   │   ├── retrieve/           多通道检索引擎
│   │   │   ├── channel/        检索通道（IntentDirected / VectorGlobal / Keyword）
│   │   │   ├── postprocessor/  后处理链（Dedup / Fusion(RRF) / Rerank）
│   │   │   └── vectorstore/    Milvus / PGVector 客户端
│   │   ├── guidance/           歧义引导（三级判定 + LLM 确认）
│   │   ├── memory/             对话记忆（加载/追加/摘要压缩）
│   │   ├── prompt/             Prompt 模板引擎（go:embed + text/template）
│   │   └── mcp/                MCP 集成（工具注册/参数提取/格式化）
│   │
│   ├── admin/                  # 管理后台 REST API（全部 9 组接口）
│   ├── user/                   # 用户认证（登录/注册/Token 管理）
│   ├── ingestion/              # 文档入库 Pipeline（Fetcher → Parser → Chunker → Indexer）
│   └── knowledge/              # 知识库 CRUD
│
├── prompts/                    # 14 个 .st Prompt 模板（go:embed 编译进二进制）
├── migrations/                 # 8 张 PostgreSQL DDL
└── configs/
    └── config.yaml             # 应用配置（完全兼容 Java yaml 结构）
```

## 二、核心技术栈

| 领域 | 选择 | 原因 |
|------|------|------|
| Agent 编排 | **tRPC-Agent-Go GraphAgent** | 替代硬编码 Pipeline，StateGraph + 条件边 + 短路路由 |
| HTTP 层 | **Gin** | 社区最成熟，性能最高 |
| LLM SDK | tRPC-Agent-Go **model/openai** | OpenAI 兼容协议，覆盖百炼/硅基/Ollama |
| ORM | **GORM** + **gen** | MyBatis-Plus → GORM，gen 保证类型安全 |
| 分布式组件 | **go-redis** | Redisson → go-redis，配置结构一致 |
| 向量检索 | **Milvus Go SDK** + **pgvector** | 官方 Go SDK |
| 鉴权 | **golang-jwt** | Sa-Token → JWT middleware |
| 可观测性 | **OpenTelemetry**（框架内置） | 替代 AOP @RagTraceNode |

## 三、核心链路：GraphAgent StateGraph

用 tRPC-Agent-Go 的 StateGraph 替代 Java 版的硬编码 8 阶段 Pipeline：

```
                    ┌──────────────┐
                    │   prepare    │  (加载记忆 + 追加消息)
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   rewrite    │  (LLM 改写 + 拆分子问题)
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   classify   │  (LLM 意图分类)
                    └──────┬───────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌──────────┐    ┌──────────────┐  ┌──────────┐
   │ guidance │    │ system_only  │  │ retrieve │
   │(短路返回) │    │ (短路返回)    │  │(正常检索) │
   └────┬─────┘    └──────┬───────┘  └────┬─────┘
        │                 │                │
        ▼                 ▼           ┌────▼─────┐
   ┌──────────┐    ┌──────────┐      │  empty?   │
   │  finish   │    │  finish   │      │(短路返回)  │
   └──────────┘    └──────────┘      └────┬─────┘
                                         │
                                   ┌─────▼──────┐
                                   │    plan    │  (场景规划 KB/MCP/MIXED)
                                   └─────┬──────┘
                                         │
                                   ┌─────▼──────┐
                                   │  respond   │  (LLM 流式答案生成)
                                   └─────┬──────┘
                                         │
                                   ┌─────▼──────┐
                                   │   finish   │  (落库 + 清理)
                                   └────────────┘
```

### 条件路由规则（和 Java 版完全一致）

| 路由点 | Go 实现 | 对应 Java 代码 |
|--------|---------|---------------|
| classify→guidance | `state.bool("ambiguity_detected")` | `handleGuidance()` 返回 true |
| classify→system_only | `state.bool("all_system_only")` | `handleSystemOnly()` 返回 true |
| classify→retrieve | 以上不满足 | 默认路径 |
| retrieve→empty | `state.bool("retrieval_empty")` | `handleEmptyRetrieval()` 返回 true |
| retrieve→plan | 有结果 | 默认路径 |

## 四、模型路由与熔断

### 架构

```
ChatService (外观)
    │
    ▼
ModelRouter (路由)
    ├── 配置优先级排序
    ├── 跳过熔断模型
    └── 选择第一个健康候选
    │
    ▼
CircuitBreaker (三态熔断器, 每个模型独立)
    ┌─────────┐  2次失败  ┌──────┐  30s后  ┌──────────┐
    │ CLOSED  │ ────────→ │ OPEN │ ────────→│HALF_OPEN │
    │(正常使用) │ ←─────── │(拒绝) │ ←─────── │(放行探测) │
    └─────────┘  恢复成功  └──────┘  再失败   └──────────┘
```

### 流式调用的首包探测

```go
func (s *ChatService) StreamChat(ctx, req, callback) error {
    for _, target := range router.SelectCandidates() {
        probe := NewFirstPacketProbe(60 * time.Second)
        err := doStream(ctx, target, req, probe.Wrap(callback))
        if err == nil { breaker.MarkSuccess(); return nil }
        breaker.MarkFailure() // 自动降级到下一个候选
    }
    return ErrAllModelsFailed
}
```

### 配置（和 Java yaml 完全一致）

```yaml
ai:
  providers:
    bailian:       {url, api_key, endpoints}
    siliconflow:   {url, api_key, endpoints}
    ollama:        {url}
    aihubmix:      {url, api_key, endpoints}
  chat:
    default_model: qwen-plus
    deep_thinking_model: qwen3-max
    candidates:    [{id, provider, model, priority, supports_thinking}]
  selection:
    failure_threshold: 2
    open_duration_ms: 30000
```

## 五、多通道检索引擎

### 架构

```
MultiChannelEngine
    │
    ├── [并行] filterEnabled(SearchContext) → executeChannels
    │    ├── IntentDirectedSearchChannel  (精确, priority=1)
    │    │    → 意图绑定 Collection → 向量检索
    │    ├── VectorGlobalSearchChannel    (兜底, priority=10)
    │    │    → 全库 Collection → 逐库 Fan-out 检索
    │    └── KeywordSearchChannel         (关键词, priority=5)
    │         → ES ik_max_word 解析 → BM25 + 向量混合
    │
    ├── [合并] 所有通道结果汇总
    │
    └── [串行] executePostProcessors
         ├── DedupPostProcessor   (按 ID 去重, 保留高优通道)
         ├── FusionPostProcessor  (RRF 倒数名次融合, k=60)
         └── RerankPostProcessor  (BaiLian Rerank API 精排)
```

### 通道启用判定

| 条件 | 效果 |
|------|------|
| 无 KB 意图 | 仅启用 VectorGlobal（全库兜底） |
| 最高分 < 0.6 | 启用 IntentDirected + VectorGlobal |
| 单意图且分 < 0.8 | 启用 IntentDirected + VectorGlobal |
| 意图明确（≥0.8） | 仅启用 IntentDirected |
| intentDirected.enabled=false | 强制启用 VectorGlobal |
| keyword.enabled=true | 追加 KeywordSearchChannel |

## 六、意图识别

### 架构

```
IntentTreeLoader
    ├── Redis 缓存 (key: "ragent:intent:tree", 7天 TTL)
    ├── MySQL fallback (t_intent_node 表)
    └── 树构建 (parentId 组装 + fillFullPath)

IntentClassifier
    ├── 获取所有叶子节点
    ├── 构造 prompt (intent-classifier.st)
    ├── LLM 调用 (temperature=0.1)
    └── JSON 解析 [{"id","score","reason"}]

IntentResolver
    ├── 过滤 score >= 0.35
    ├── 每子问题最多 3 个意图
    ├── 全局最多 3 个 (capTotalIntents)
    └── 分组: KB / MCP / SYSTEM
```

## 七、对话记忆

### 架构

```
ConversationMemory (加载+追加)
    │
    ├── [并行] goroutine pool
    │    ├── SummaryService  (加载最新摘要, system role)
    │    └── JDBCStore       (加载最近 8 轮原文, user/assistant)
    │
    ├── 合并: [摘要(system)] + [历史(user+assistant)]
    │
    └── append() → 异步写入 DB

SummaryService (摘要压缩)
    ├── 条件: assistant 回复后 && 消息数 >= 8
    ├── 增量: 上次摘要 afterId → cutoffId 之间
    ├── LLM 压缩 (conversation-summary.st, max200字)
    ├── go-redis 分布式锁 (userId:conversationId)
    └── 持久化: t_conversation_summary
```

### 隔离设计

所有存储操作强制带 `conversation_id + user_id` 双重条件。

## 八、MCP 工具集成

### 架构

```
bootstrap (MCP Client)              mcp-server (独立进程 :9099)
┌────────────────────────┐          ┌────────────────────────┐
│ McpToolRegistry         │          │ MCP SSE Server          │
│   Map<toolId, Executor> │          │   SalesTool             │
│                         │  ◄───→   │   TicketTool            │
│ LLMParamExtractor       │   MCP   │   WeatherTool           │
│   → 提取参数            │  Protocol│                          │
│                         │          │                          │
│ tRPC-Agent-Go           │          │ tRPC-Agent-Go           │
│  mcp.NewMCPToolSet()   │          │  mcp.NewServer()        │
└────────────────────────┘          └────────────────────────┘
```

## 九、SSE 流式响应协议

与前端协议完全一致（前端零改动）：

| 事件 | 数据结构 | 说明 |
|------|---------|------|
| `meta` | `{"conversationId":"...","taskId":"..."}` | 会话元信息 |
| `message` | `{"type":"response","delta":"文字"}` | LLM 回答正文（逐字） |
| `message` | `{"type":"think","delta":"推理..."}` | 深度思考过程 |
| `finish` | `{"messageId":"...","title":"..."}` | 回答落库后 |
| `done` | `"[DONE]"` | 流结束 |
| `reject` | `{"type":"response","delta":"系统繁忙..."}` | 限流拒绝 |
| `cancel` | `{"messageId":"..."}` | 用户取消 |

## 十、分布式限流

Go 版用 `go-redis + Lua` 完整复刻 Java 版 `FairDistributedRateLimiter`：

```
FairLimiter
    ├── ZSET 排队 (RScoredSortedSet → ZADD/ZRANGEBYSCORE)
    ├── Lua 原子出队 (queue_claim_atomic.lua, 和 Java 版完全一致)
    ├── SETNX Semaphore (许可自动过期防死锁)
    └── Pub/Sub 跨实例唤醒 (PollNotifier → goroutine + channel)
```

## 十一、文档入库 Pipeline

```
上传 → Fetcher → Parser → Chunker → Enricher → Indexer
        (获取)    (解析)   (分块)   (元数据)   (向量化)
                     │
                  MinerU API (PDF)
                  Tika  替代 (Word/Markdown)
```

## 十二、设计模式

| 模式 | 应用位置 |
|------|---------|
| 策略模式 | SearchChannel 接口, PostProcessor 接口 |
| 模板方法 | GraphAgent 节点定义 |
| 注册表 | McpToolRegistry, NodeRegistry |
| 责任链 | PostProcessor 链, 模型降级链 |
| 观察者 | StreamCallback → channel |
| 装饰器 | FirstPacketProbe 首包探测 |

## 十三、配置结构

Go 版配置文件 `configs/config.yaml`，结构命名和 Java 版 `application.yaml` 保持一致，字段名从 kebab-case 转为 snake_case：

```yaml
server:
  port: 9090
  context_path: /api/ragent

spring:
  datasource: {driver, url, username, password}
  data:
    redis: {host, port, password}

milvus: {uri}

rag:
  vector: {type: pg|milvus}
  keyword: {type: none|es}
  default: {collection_name, dimension, metric_type, sse_timeout_ms}
  query_rewrite: {enabled: true}
  rerank: {enabled: true}
  rate_limit: {global: {enabled, max_concurrent, max_wait_seconds, ...}}
  memory: {history_keep_turns, summary_start_turns, summary_enabled, ...}
  search:
    channels:
      vector_global: {confidence_threshold, top_k_multiplier, candidate_budget}
      intent_directed: {enabled, min_intent_score, top_k_multiplier}
      keyword: {enabled, mode, top_k_multiplier}
    fusion: {strategy, rrf_k, rerank_candidate_limit}

ai:
  providers: {ollama, bailian, siliconflow, aihubmix}
  chat: {default_model, deep_thinking_model, candidates}
  embedding: {default_model, candidates}
  rerank: {default_model, candidates}
  selection: {failure_threshold, open_duration_ms}
  stream: {message_chunk_size}
```
