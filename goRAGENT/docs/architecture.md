# goRAGENT 架构设计文档

> 版本: v2.0.0 | 日期: 2026-07-17 | 企业级分层架构

## 一、项目结构

```
goRAGENT/
├── cmd/server/main.go              # 入口（64 行）：加载配置 → bootstrap → 启动
│
├── internal/
│   ├── bootstrap/                  # 依赖装配（唯一知道所有具体实现的地方）
│   │   ├── bootstrap.go            # App 结构 + New(cfg) + Run() + 优雅关闭
│   │   └── probe.go                # 启动自检（DB/Redis/Milvus/Embedding/LLM）
│   │
│   ├── router/                     # 路由注册（模块化拆分，零 gorm import）
│   │   ├── router.go               # Register() + Deps 结构 + 5 个 register*() 函数
│   │   └── health.go               # HealthHandler + SettingsHandler
│   │
│   ├── handler/                    # HTTP 层（薄 Controller：只 bind → svc → render）
│   │   ├── httpx/httpx.go          # 分页参数解析 + 统一错误渲染
│   │   ├── admin/                  # 管理后台 12 个 handler（零 gorm，零业务逻辑）
│   │   ├── auth/handler.go         # 登录/注册/当前用户
│   │   ├── chat/handler.go         # SSE 流式对话
│   │   └── session/handler.go      # 会话列表/重命名/删除/消息/反馈
│   │
│   ├── service/                    # 业务逻辑层（全部接口化，零 gin/gorm import）
│   │   ├── admin/                  # 管理后台 11 个 domain service（接口+实现）
│   │   ├── auth/                   # AuthService + PasswordHasher 接口
│   │   ├── rag/                    # RAG 编排（Pipeline/Memory/Intent/Retrieval…）
│   │   ├── ingestion/              # 文档入库流水线
│   │   └── mcp/                    # MCP 工具集
│   │
│   ├── repository/                 # 数据访问接口（按领域拆分，零业务逻辑）
│   │   ├── repository.go           # Repositories 聚合结构
│   │   ├── *.go                    # 14 个接口（User/Knowledge/Conversation/…）
│   │   └── mysql/                  # GORM 实现 + notDeleted scope + sqlite 测试
│   │
│   ├── model/                      # 领域模型（纯数据结构，零 internal import）
│   │   ├── *.go                    # DO（t_user/t_knowledge_base/… 共 14 张表）
│   │   ├── *_dto.go                # 请求 DTO / 响应 VO
│   │   ├── page.go                 # PageQuery / PageResult 统一分页
│   │   └── consts.go               # 业务常量（状态/角色/向量维度/…）
│   │
│   ├── middleware/                 # HTTP 中间件（auth / ratelimit / userctx）
│   └── config/config.go            # 纯配置加载
│
├── pkg/                            # 可复用公共库（不依赖 internal/ —— 存量 4 处待解耦）
│   ├── errs/                       # 统一错误类型（AppError + 分级错误码 + Wrap）
│   ├── response/                   # 统一 JSON 响应体（Success/Failure/FromError）
│   ├── llm/                        # ChatService / ModelRouter / CircuitBreaker
│   ├── embedding/ / rerank/        # AI 服务客户端
│   ├── milvus/store.go             # 向量库客户端
│   ├── prompt/loader.go            # Prompt 模板引擎（go:embed）
│   ├── sse/ / jwt/ / snowflake/ / logx/ / mineru/
│
├── docker/                         # docker-compose + nginx + init.sql
└── docs/                           # 架构 / 开发规范 / 重构报告
```

### 分层职责与依赖方向

| 层 | 包 | 职责 | 可依赖 |
|---|---|---|---|
| 入口 | `cmd/server/` | 加载配置，调用 bootstrap 启动 | bootstrap, config |
| 装配 | `internal/bootstrap/` | 创建所有具体实现，注入 handler | 所有层 |
| 路由 | `internal/router/` | 模块化路由注册，挂载 handler | handler, middleware |
| HTTP | `internal/handler/` | 参数绑定/校验 → 调 service → 渲染响应 | service 接口, model, httpx |
| 业务 | `internal/service/` | 业务编排、事务边界、跨资源协调 | repository 接口, model, pkg |
| 数据 | `internal/repository/` | 单表/聚合数据访问，context 透传 | model, gorm（仅 mysql/ 子包） |
| 模型 | `internal/model/` | DO/DTO/VO/常量，纯数据 | 标准库 |
| 中间件 | `internal/middleware/` | HTTP 拦截器 | pkg |
| 配置 | `internal/config/` | 环境变量加载 | 标准库 |
| 公共 | `pkg/` | 可复用库 | 外部 SDK |

**依赖方向**: handler → service → repository → model，严格单向。pkg 不依赖 internal（4 处存量待后续解耦）。

---

## 二、核心技术栈

| 领域 | 选择 | 原因 |
|------|------|------|
| Agent 编排 | **tRPC-Agent-Go GraphAgent** | StateGraph + 条件边 + 短路路由，灵活的 DAG 编排 |
| HTTP 层 | **Gin** | 社区最成熟，性能最高 |
| LLM SDK | tRPC-Agent-Go **model/openai** | OpenAI 兼容协议，覆盖百炼/硅基/Ollama |
| ORM | **GORM**（仅 repository 层使用） | 接口抽象，零散落 |
| 分布式组件 | **go-redis** | 高性能 Redis 客户端 |
| 向量检索 | **Milvus Go SDK** | 官方 Go SDK |
| 鉴权 | **golang-jwt** | JWT 标准实现 |
| 可观测性 | **OpenTelemetry**（框架内置） | 全链路追踪 |
| 错误处理 | **pkg/errs.AppError** | 分级错误码 + Wrap 链 + 统一渲染 |
| 日志 | **zap**（结构化） | 高性能结构化日志 |

---

## 三、核心链路：GraphAgent StateGraph

基于 tRPC-Agent-Go 的 StateGraph 构建 RAG Pipeline：

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

### 条件路由规则

| 路由点 | 条件 |
|--------|------|
| classify→guidance | `state.bool("ambiguity_detected")` |
| classify→system_only | `state.bool("all_system_only")` |
| classify→retrieve | 以上不满足 |
| retrieve→empty | `state.bool("retrieval_empty")` |
| retrieve→plan | 有结果 |

---

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

### 配置

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

---

## 五、多通道检索引擎

### 架构

```
MultiChannelEngine
    │
    ├── [并行] filterEnabled(SearchContext) → executeChannels
    │    ├── IntentDirectedSearchChannel  (精确, priority=1)
    │    ├── VectorGlobalSearchChannel    (兜底, priority=10)
    │    └── KeywordSearchChannel         (关键词, priority=5)
    │
    ├── [合并] 所有通道结果汇总
    │
    └── [串行] executePostProcessors
         ├── DedupPostProcessor   (按 ID 去重, 保留高优通道)
         ├── FusionPostProcessor  (RRF 倒数名次融合, k=60)
         └── RerankPostProcessor  (Rerank API 精排)
```

### 通道启用判定

| 条件 | 效果 |
|------|------|
| 无 KB 意图 | 仅启用 VectorGlobal（全库兜底） |
| 最高分 < 0.6 | 启用 IntentDirected + VectorGlobal |
| 单意图且分 < 0.8 | 启用 IntentDirected + VectorGlobal |
| 意图明确（≥0.8） | 仅启用 IntentDirected |

---

## 六、意图识别

```
IntentTreeLoader
    ├── Redis 缓存 (key: "ragent:intent:tree")
    ├── MySQL fallback (repository.IntentNodeRepository)
    └── 树构建

IntentClassifier
    ├── 获取所有叶子节点
    ├── LLM 调用 (temperature=0.1)
    └── JSON 解析 [{"id","score","reason"}]

IntentResolver
    ├── 过滤 score >= 0.35
    ├── 每子问题最多 3 个意图
    └── 分组: KB / MCP / SYSTEM
```

---

## 七、对话记忆

```
ConversationMemory (加载+追加)
    │
    ├── [并行] goroutine pool
    │    ├── SummaryService  (加载最新摘要, system role)
    │    └── MessageRepo     (加载最近历史)
    │
    ├── 合并: [摘要(system)] + [历史(user+assistant)]
    │
    └── append() → 异步写入 DB

SummaryService (摘要压缩)
    ├── 条件: 消息数 >= 8
    ├── 增量压缩 (LLM, max200字)
    ├── go-redis 分布式锁
    └── 持久化: t_conversation_summary
```

### 隔离设计

所有存储操作强制带 `conversation_id + user_id` 双重条件（通过 `ForUser` 方法族）。

---

## 八、MCP 工具集成

```
bootstrap (MCP Client)              mcp-server (独立进程 :9099)
┌────────────────────────┐          ┌────────────────────────┐
│ McpToolRegistry         │          │ MCP SSE Server          │
│ LLMParamExtractor       │  ◄───→   │ SalesTool / TicketTool  │
│                         │   MCP   │ WeatherTool             │
│ tRPC-Agent-Go           │  Protocol│                         │
│  mcp.NewMCPToolSet()   │          │ tRPC-Agent-Go           │
└────────────────────────┘          │  mcp.NewServer()        │
                                    └────────────────────────┘
```

---

## 九、SSE 流式响应协议

| 事件 | 数据结构 | 说明 |
|------|---------|------|
| `meta` | `{"conversationId":"...","taskId":"..."}` | 会话元信息 |
| `message` | `{"type":"response","delta":"文字"}` | LLM 回答正文（逐字） |
| `message` | `{"type":"think","delta":"推理..."}` | 深度思考过程 |
| `finish` | `{"messageId":"...","title":"..."}` | 回答落库后 |
| `done` | `"[DONE]"` | 流结束 |
| `reject` | `{"type":"response","delta":"系统繁忙..."}` | 限流拒绝 |
| `cancel` | `{"messageId":"..."}` | 用户取消 |

---

## 十、分布式限流

```
FairLimiter
    ├── ZSET 排队
    ├── Lua 原子出队
    ├── SETNX Semaphore（许可自动过期）
    └── Pub/Sub 跨实例唤醒
```

---

## 十一、文档入库 Pipeline

```
上传 → Fetcher → Parser → Chunker → Enricher → Indexer
        (获取)    (解析)   (分块)   (元数据)   (向量化)
                     │
                  MinerU API (PDF)
                  Tika  替代 (Word/Markdown)
```

---

## 十二、设计模式

| 模式 | 应用位置 |
|------|---------|
| 策略模式 | SearchChannel 接口, PostProcessor 接口 |
| 模板方法 | GraphAgent 节点定义 |
| 注册表 | McpToolRegistry |
| 责任链 | PostProcessor 链, 模型降级链 |
| 观察者 | StreamCallback → channel |
| 装饰器 | FirstPacketProbe 首包探测 |
| **工厂模式** | 全部 `NewXxx()` 构造函数返回接口 |
| **依赖注入** | bootstrap 装配 + 构造函数注入 |
| **仓储模式** | Repository 接口 + MySQL 实现 |
