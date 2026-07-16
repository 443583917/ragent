# goRAGENT 工程规范与项目重构计划

> 日期: 2026-07-17 | 目标: 对齐 Go 社区标准布局，抽取 router/pkg/model 层，保持 Java 设计模式

## 一、现状诊断

### 1.1 当前目录结构

```
goRAGENT/
├── cmd/server/
│   ├── main.go          # 入口 + 全部路由(270行) + 全部依赖装配
│   ├── health.go        # health 端点
│   └── init.go          # DB/Redis/Milvus/LLM 初始化
├── internal/
│   ├── admin/           # 管理后台 handler（混合了 controller + service 逻辑）
│   ├── config/          # 环境变量配置
│   ├── framework/       # 基础组件
│   │   ├── jwt/         # JWT 鉴权
│   │   ├── logx/        # zap 日志
│   │   ├── ratelimit/   # 分布式限流
│   │   ├── response/    # 统一响应体
│   │   ├── snowflake/   # 分布式 ID
│   │   ├── sse/         # SSE 事件协议
│   │   └── userctx/     # 用户上下文
│   ├── infra/           # 外部服务客户端
│   │   ├── embedding/   # BGE-M3 HTTP
│   │   ├── llm/         # LLM 模型路由+ChatService
│   │   ├── mineru/      # MinerU 文档解析
│   │   └── rerank/      # BGE-M3 Rerank
│   ├── ingestion/       # 入库 Pipeline（Fetcher→Parser→Chunker→Indexer→Engine）
│   ├── rag/             # 核心 RAG 逻辑（混合了 model + service + handler + prompt）
│   │   ├── guidance/    # 歧义引导
│   │   ├── intent/      # 意图分类+解析
│   │   ├── mcp/         # MCP 客户端+注册表+执行器
│   │   ├── memory/      # 对话记忆+摘要
│   │   ├── pipeline/    # 8 阶段 Pipeline + SSE Handler
│   │   ├── prompt/      # go:embed 模板引擎
│   │   ├── retrieve/    # 多通道检索
│   │   │   ├── postprocessor/  # 后处理器
│   │   │   └── vectorstore/    # Milvus
│   │   ├── rewrite/     # 查询改写+同义词
│   │   ├── *.go         # knowledge/audit/trace/sample_question DO 模型 + session handler
│   └── user/            # 注册/登录 handler
├── docker/              # Docker Compose + Nginx + init.sql
└── docs/                # 文档
```

### 1.2 核心问题

| 问题 | 详情 |
|------|------|
| **router 缺失** | main.go 270 行路由，无独立 router 包 |
| **model 散落** | DO 模型混在 `internal/rag/*.go` 和 `internal/user/` 和 `internal/admin/` 中 |
| **handler 臃肿** | admin handler 文件同时包含 controller 方法 + 业务逻辑 + DB 查询 |
| **infra/pkg 混淆** | framework 下的 sse/jwt/snowflake 是通用工具，infra 下的 llm/embedding 是外部服务 |
| **import 路径过长** | 之前 `github.com/nageoffer/ragent/goRAGENT/...` 已改为 `goRAGENT/...` |
| **Java 设计模式** | 策略/责任链/外观模式已实现但隐藏在不规范的结构中 |

---

## 二、目标架构

### 2.1 新目录树

```
goRAGENT/
├── cmd/server/
│   └── main.go                        # 入口，仅 bootstrap（~50行）
│
├── internal/                           # 私有应用代码
│   ├── router/
│   │   └── router.go                  # 全部路由注册（单一职责）
│   │
│   ├── handler/                        # HTTP 层（Controller）
│   │   ├── admin/                     # 管理后台 handlers
│   │   │   ├── dashboard.go
│   │   │   ├── knowledge_base.go
│   │   │   ├── document.go
│   │   │   ├── chunk.go
│   │   │   ├── intent_tree.go
│   │   │   ├── query_term_mapping.go
│   │   │   ├── ingestion_task.go
│   │   │   ├── trace.go
│   │   │   ├── audit_log.go
│   │   │   ├── sample_question.go
│   │   │   ├── user_mgmt.go
│   │   │   └── settings.go
│   │   ├── chat/                      # 对话 Handlers
│   │   │   └── handler.go            # SSE StreamChat + StopTask
│   │   └── auth/                      # 认证 Handlers
│   │       └── handler.go            # Login + Register + CurrentUser
│   │
│   ├── service/                        # 业务逻辑层（Service）
│   │   ├── rag/                       # RAG 核心服务
│   │   │   ├── pipeline.go           # 8 阶段 Pipeline
│   │   │   ├── intent_service.go     # 意图分类编排
│   │   │   ├── rewrite_service.go    # 查询改写编排
│   │   │   ├── guidance_service.go   # 歧义引导编排
│   │   │   ├── memory_service.go     # 会话记忆编排
│   │   │   └── retrieval_service.go  # 检索编排
│   │   ├── ingestion/
│   │   │   ├── engine.go
│   │   │   ├── fetcher.go
│   │   │   ├── parser.go
│   │   │   ├── chunker.go
│   │   │   └── indexer.go
│   │   ├── mcp/
│   │   │   ├── client.go
│   │   │   ├── registry.go
│   │   │   ├── extractor.go
│   │   │   ├── formatter.go
│   │   │   └── executor.go
│   │   └── admin/                     # 管理后台业务服务
│   │       ├── audit_service.go
│   │       └── trace_service.go
│   │
│   ├── repository/                     # 数据访问层（Repository/DAO）
│   │   └── mysql/
│   │       ├── user_repo.go
│   │       ├── conversation_repo.go
│   │       ├── knowledge_repo.go
│   │       ├── document_repo.go
│   │       ├── chunk_repo.go
│   │       ├── ingestion_task_repo.go
│   │       ├── trace_repo.go
│   │       ├── audit_repo.go
│   │       └── sample_question_repo.go
│   │
│   ├── model/                          # 领域模型 / DO（纯数据结构）
│   │   ├── user.go
│   │   ├── conversation.go
│   │   ├── knowledge.go
│   │   ├── document.go
│   │   ├── chunk.go
│   │   ├── ingestion_task.go
│   │   ├── intent.go
│   │   ├── trace.go
│   │   ├── audit.go
│   │   ├── sample_question.go
│   │   └── common.go                  # 分页/状态常量
│   │
│   ├── middleware/                     # HTTP 中间件
│   │   ├── auth.go                    # JWT 鉴权中间件
│   │   ├── ratelimit.go              # 限流中间件
│   │   └── userctx.go                # 用户上下文注入
│   │
│   ├── config/
│   │   └── config.go                  # 环境变量配置
│   │
│   └── bootstrap/                     # 依赖装配
│       └── wire.go                    # 依赖注入装配（创建 DB/Redis/Milvus/LLM 等）
│
├── pkg/                                # 可复用公共库（无内部依赖）
│   ├── llm/
│   │   ├── chat.go
│   │   └── router.go
│   ├── embedding/
│   │   └── client.go
│   ├── rerank/
│   │   └── client.go
│   ├── mineru/
│   │   └── client.go
│   ├── milvus/
│   │   └── store.go
│   ├── prompt/
│   │   ├── loader.go
│   │   └── prompts/            # go:embed *.st
│   ├── sse/
│   │   ├── emitter.go
│   │   └── event.go
│   ├── jwt/
│   │   └── jwt.go
│   ├── snowflake/
│   │   └── snowflake.go
│   ├── logx/
│   │   └── log.go
│   └── response/
│       └── response.go
│
├── docker/
│   ├── docker-compose.yml
│   ├── nginx/
│   └── init.sql
│
└── docs/
```

### 2.2 分层职责

| 层 | 目录 | 职责 | 可依赖 |
|------|------|------|------|
| **cmd** | `cmd/server/` | 入口，调用 bootstrap 启动 | bootstrap, config |
| **router** | `internal/router/` | 路由注册，挂载 handler 到 gin.Engine | handler, middleware |
| **handler** | `internal/handler/` | HTTP 请求解析/校验/响应，调用 service | service, model |
| **service** | `internal/service/` | 业务逻辑编排，调用 repository + pkg | repository, model, pkg |
| **repository** | `internal/repository/` | 数据访问，封装 GORM/SQL | model |
| **model** | `internal/model/` | 纯数据结构（DO/VO），无逻辑 | 无 |
| **middleware** | `internal/middleware/` | HTTP 拦截器 | pkg |
| **config** | `internal/config/` | 环境变量加载 | 无 |
| **bootstrap** | `internal/bootstrap/` | 依赖装配（创建并注入所有组件） | 所有层 |
| **pkg** | `pkg/` | 可复用库，不依赖 `internal/` | 外部 SDK |

### 2.3 Import 规范

- **内部引用**: `goRAGENT/internal/{layer}` → `goRAGENT/pkg/{lib}`
- **pkg 不依赖 internal**: pkg 下代码零 internal import，保证可复用
- **model 零依赖**: model 层不 import 任何项目内部包，纯 struct

---

## 三、Java 设计模式 → Go 对照

| Java 设计模式 | Java 位置 | Go 实现 | Go 位置（新） | 保持 |
|:--|:--|:--|:--|:--:|
| **策略模式** | SearchChannel 接口 | SearchChannel interface | pkg/milvus/store.go | ✅ |
| **责任链** | PostProcessor 链 | PostProcessor interface + 链式调用 | internal/service/rag/retrieval_service.go | ✅ |
| **外观模式** | RetrievalEngine 包装 MultiChannelEngine | RetrievalEngine struct | internal/service/rag/retrieval_service.go | ✅ |
| **模板方法** | Pipeline 8 阶段 | SimplePipeline.Execute() 顺序调用 | internal/service/rag/pipeline.go | ✅ |
| **工厂模式** | NewXxx 构造函数 | NewXxx() 函数 | 各层 | ✅ |
| **观察者** | SSE StreamChatEventHandler | sseCallback + StreamCallback | internal/handler/chat/handler.go | ✅ |
| **单例** | Spring @Component | init() 中创建一次 + 全局持有 | internal/bootstrap/wire.go | ✅ |
| **适配器** | RerankerAdapter | RerankerAdapter 函数 | pkg/rerank/client.go | ✅ |
| **装饰器** | MetadataEnrichment 先于 Dedup 执行 | PostProcessor.Order() 排序 | internal/service/rag/retrieval_service.go | ✅ |

**结论**: Go 版已通过接口（interface）和组合（struct embedding）自然保持了 Java 的设计模式，无需额外改动。

---

## 四、文件迁移对照表

### 4.1 pkg/ 迁移（框架/工具层）

| 原路径 | → 新路径 |
|------|------|
| `internal/infra/llm/chat.go` | `pkg/llm/chat.go` |
| `internal/infra/llm/router.go` | `pkg/llm/router.go` |
| `internal/infra/embedding/embedding.go` | `pkg/embedding/client.go` |
| `internal/infra/rerank/*.go` | `pkg/rerank/client.go` |
| `internal/infra/mineru/mineru.go` | `pkg/mineru/client.go` |
| `internal/rag/retrieve/vectorstore/milvus.go` | `pkg/milvus/store.go` |
| `internal/rag/prompt/loader.go` | `pkg/prompt/loader.go` |
| `internal/rag/prompt/prompts/` | `pkg/prompt/prompts/` |
| `internal/framework/sse/emitter.go` | `pkg/sse/emitter.go` |
| `internal/framework/sse/event.go` | `pkg/sse/event.go` |
| `internal/framework/jwt/middleware.go` | `pkg/jwt/jwt.go` |
| `internal/framework/snowflake/*.go` | `pkg/snowflake/snowflake.go` |
| `internal/framework/logx/*.go` | `pkg/logx/log.go` |
| `internal/framework/response/response.go` | `pkg/response/response.go` |

### 4.2 internal/model/ 迁移（领域模型）

| 原路径 | → 新路径 |
|------|------|
| `internal/rag/knowledge.go` (KnowledgeBaseDO/DocumentDO/ChunkDO/IngestionTaskDO) | `internal/model/knowledge.go` + `internal/model/document.go` + `internal/model/chunk.go` + `internal/model/ingestion_task.go` |
| `internal/rag/trace.go` | `internal/model/trace.go` |
| `internal/rag/audit.go` | `internal/model/audit.go` |
| `internal/rag/sample_question.go` | `internal/model/sample_question.go` |
| `internal/rag/intent/node.go` (IntentNodeDO) | `internal/model/intent.go` |
| `internal/user/user.go` (UserDO) | `internal/model/user.go` |
| `internal/rag/memory/memory.go` (ConversationDO/MessageDO) | `internal/model/conversation.go` |

### 4.3 internal/service/ 迁移（业务逻辑）

| 原路径 | → 新路径 |
|------|------|
| `internal/rag/pipeline/pipeline.go` | `internal/service/rag/pipeline.go` |
| `internal/rag/intent/classifier.go + resolver.go + loader.go` | `internal/service/rag/intent_service.go` |
| `internal/rag/rewrite/rewrite.go + term_mapping.go` | `internal/service/rag/rewrite_service.go` |
| `internal/rag/guidance/guidance.go` | `internal/service/rag/guidance_service.go` |
| `internal/rag/memory/memory.go + summary.go` | `internal/service/rag/memory_service.go` |
| `internal/rag/retrieve/engine.go + channels.go + channel.go + postprocessors.go` | `internal/service/rag/retrieval_service.go` |
| `internal/rag/retrieve/postprocessor/metadata_enrich.go` | `internal/service/rag/metadata_enrich.go` |
| `internal/ingestion/` (all 6 files) | `internal/service/ingestion/` |
| `internal/rag/mcp/` (all 5 files) | `internal/service/mcp/` |

### 4.4 internal/handler/ 迁移（Controller）

| 原路径 | → 新路径 |
|------|------|
| `internal/admin/admin.go` | `internal/handler/admin/` (拆到各文件) |
| `internal/admin/dashboard.go` | `internal/handler/admin/dashboard.go` |
| `internal/admin/knowledge_base.go` | `internal/handler/admin/knowledge_base.go` |
| `internal/admin/document.go` | `internal/handler/admin/document.go` |
| `internal/admin/chunk.go` | `internal/handler/admin/chunk.go` |
| `internal/admin/intent_tree.go` | `internal/handler/admin/intent_tree.go` |
| `internal/admin/query_term_mapping.go` | `internal/handler/admin/query_term_mapping.go` |
| `internal/admin/ingestion_task.go` | `internal/handler/admin/ingestion_task.go` |
| `internal/admin/trace.go` | `internal/handler/admin/trace.go` |
| `internal/admin/audit_log.go` | `internal/handler/admin/audit_log.go` |
| `internal/admin/sample_question.go` | `internal/handler/admin/sample_question.go` |
| `internal/admin/user_mgmt.go` | `internal/handler/admin/user_mgmt.go` |
| `internal/rag/pipeline/handler.go` | `internal/handler/chat/handler.go` |
| `internal/user/user.go` (handler 部分) | `internal/handler/auth/handler.go` |

### 4.5 internal/middleware/ 迁移

| 原路径 | → 新路径 |
|------|------|
| `internal/framework/jwt/middleware.go` | `internal/middleware/auth.go` (JWT 逻辑抽到 pkg/jwt) |
| `internal/framework/ratelimit/limiter.go` | `internal/middleware/ratelimit.go` |
| `internal/framework/userctx/` | `internal/middleware/userctx.go` |

### 4.6 internal/bootstrap/ + internal/router/

| 原路径 | → 新路径 |
|------|------|
| `cmd/server/init.go` | `internal/bootstrap/wire.go` |
| `cmd/server/health.go` | `internal/handler/health.go` |
| `cmd/server/main.go` 路由部分 | `internal/router/router.go` |
| `cmd/server/main.go` 剩余 | `cmd/server/main.go` (~50行) |

---

## 五、执行任务

### Task 1: 创建新目录结构 + 移动 pkg/ 文件
### Task 2: 创建 internal/model/ + 移动 DO 模型
### Task 3: 创建 internal/service/ + 移动业务逻辑
### Task 4: 创建 internal/handler/ + 移动 Controller
### Task 5: 创建 internal/middleware/ + internal/bootstrap/ + internal/router/
### Task 6: 全局 import 路径更新（goRAGENT/internal/xxx → goRAGENT/pkg/xxx 等）
### Task 7: 更新测试文件 import 路径
### Task 8: 验证：go build ./... + go test ./... + 前后端联调
