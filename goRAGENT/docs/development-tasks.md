# goRAGENT 开发任务文档

> 版本: v1.0.0 | 日期: 2026-07-16
> 总工期: 约 11 周 | 5 个 Phase | 每 Phase 独立验收

## Phase 优先级总览

| Phase | 目标 | 工期 | 依赖 |
|:-----:|------|:----:|------|
| P1 | 骨架搭建 + 基础设施 | 2 周 | 无 |
| P2 | 核心 RAG 链路 | 3 周 | P1 |
| P3 | 摘要 + 歧义 + 入库 | 2 周 | P2 |
| P4 | 限流 + Trace + 管理后台 | 2 周 | P1, P2 |
| P5 | 测试 + 性能 + 文档 | 2 周 | P1-P4 |

---

## 当前迭代：意图树加载 + LLM 分类（2026-07-17 设计定稿，✅ 已完成并验收）

> 范围：完整链路（Loader → Classifier → Resolver → Pipeline 接入）+ 管理后台意图树 CRUD 实装。
> 决策：`t_intent_node` 表采用数值 level/kind、独立 id 主键、补 sort_order；
> 整问题作为单一子问题分类（一次 LLM 调用，免线程池）；MCP 意图仅流转不执行（P2）。

| # | 任务 | 对应原任务 | 产出文件 |
|:--|------|:--------:|---------|
| I-1 | 重建 t_intent_node 表（对齐前端契约） | 1.4 | `docker/init.sql` |
| I-2 | IntentNode 模型 + IntentNodeDO + Kind/Level 枚举 | 2.6 | `internal/rag/intent/node.go` |
| I-3 | TreeLoader：Redis(`ragent:intent:tree`, 7天) → MySQL fallback，buildTree/fullPath 纯函数，ClearCache | 2.6 | `internal/rag/intent/loader.go` + `loader_test.go` |
| I-4 | Classifier：叶子序列化 `{intent_list}` → LLM(temp=0.1/topP=0.3) → JSON 容错解析(code fence/results 包裹/未知 id) | 2.7 | `internal/rag/intent/classifier.go` + `classifier_test.go` |
| I-5 | Resolver：score≥0.35 过滤 + cap 3 → 转换 retrieve.SubQuestionIntent | 2.8 | `internal/rag/intent/resolver.go` + `resolver_test.go` |
| I-6 | Pipeline 接入：检索前填充 SearchContext.Intents（失败降级空意图） | 2.28 | `internal/rag/pipeline/pipeline.go` |
| I-7 | 管理后台 intent-tree CRUD 实装（7 端点，写后清缓存） | 4.10 | `internal/admin/intent_tree.go` |
| I-8 | main.go 装配 + 空壳路由替换 | 1.8 | `cmd/server/main.go` |
| I-9 | 意图单测补全（树构建/解析容错/阈值 cap/VO 转换） | 5.2 | `internal/rag/intent/*_test.go` |

### 当前迭代验收标准

```bash
# 1. 单测全绿
go test ./internal/rag/intent/... ./internal/admin/... -count=1

# 2. 意图定向检索激活
# 造意图节点数据后提问命中意图 → 日志出现 "执行意图定向检索"

# 3. 管理后台意图树
# GET /api/ragent/intent-tree/trees 返回树形 JSON（前端页面可编辑）
# 写操作后 Redis key ragent:intent:tree 被清除

# 4. 降级验证
# 停 Redis / 清空表 / LLM 挂掉 → 问答不中断，回退全局向量检索
```

---

## 迭代路线图（全量功能盘点）

> 依据：8 阶段 Pipeline / 记忆 / MCP / 检索 / 限流 / 管理后台全量盘点。
> 原则：先修核心体验（记忆/会话），再补检索质量，再扩平台能力。
> 每个迭代独立验收，完成后在此标记 ✅ 并同步 handover.md。

### M1：对话记忆真实装 + 会话管理（P0 · 核心体验）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M1-1 | 记忆加载实装：按 conversationId 取 historyKeepTurns×2 条，去前导 assistant，保留 user/assistant 对 | `internal/rag/memory/memory.go` + 测试 |
| M1-2 | 消息持久化：user 消息进 Pipeline 时落库；assistant 消息 OnComplete 落库 | `memory.go`, `pipeline/handler.go` |
| M1-3 | 会话 createOrUpdate：首条消息建 t_conversation（标题=问题截断，LLM 标题 M3 再做） | `memory.go` |
| M1-4 | 会话管理 API 实装：列表/重命名/删除/消息历史 | `internal/rag/session.go` + 测试 |
| M1-5 | 消息反馈实装（点赞/踩，同步写库即可，不引 MQ） | `session.go` |

验收：多轮对话第二轮能引用第一轮内容（指代消解）；前端历史会话列表/消息回放可用；反馈落库。

### M2：查询改写 + 同义词 + 空检索短路（P1 · 检索质量）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M2-1 | 关键词映射 CRUD 实装（t_query_term_mapping） | `internal/admin/` |
| M2-2 | 同义词归一化 | `internal/rag/rewrite/term_mapping.go` |
| M2-3 | LLM 查询改写 + 子问题拆分（JSON 输出，失败回退规则拆分按 ？?。；; ） | `internal/rag/rewrite/` |
| M2-4 | 意图解析升级：按子问题并行分类 + capTotalIntents 保底分配 | `internal/rag/intent/resolver.go` |
| M2-5 | 空检索短路：未命中文档直接返回提示语，不走 LLM | `internal/rag/pipeline/pipeline.go` |
| M2-6 | Prompt 场景规划（KB_ONLY/EMPTY + 意图节点自带模板覆盖） | `internal/rag/prompt/planner.go` |

### M3：摘要压缩 + 歧义引导 + SYSTEM 短路（P2 · 对话增强）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M3-1 | 摘要压缩：用户消息数≥summaryStartTurns 异步触发，Redis SETNX 锁，LLM 压缩落 t_conversation_summary | `internal/rag/memory/summary.go` |
| M3-2 | 摘要拼接进历史（system 角色 summary-wrapper 包裹置顶） | `memory.go` |
| M3-3 | LLM 会话标题生成 | `memory.go` |
| M3-4 | 歧义引导：分数比值≥阈值或边界区间 LLM 二次确认 → 推送选项短路 | `internal/rag/guidance/` |
| M3-5 | SYSTEM 意图短路直答（支持节点模板覆盖） | `pipeline.go` |

### M4：知识库/文档管理 + 入库 Pipeline（P2 · 内容供给）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M4-1 | 知识库 CRUD 实装 | `internal/admin/` |
| M4-2 | 文档上传/列表/详情/启停用/删除 | `internal/admin/` + 文件存储 |
| M4-3 | 入库 Pipeline：解析（MinerU 接线）→分块→嵌入→Milvus 索引 | `internal/ingestion/` |
| M4-4 | Chunk 管理 CRUD + 启停用 | `internal/admin/` |
| M4-5 | 入库任务监控 API | `internal/admin/` |
| M4-6 | 检索元数据富化后处理器（回表补 docId/docName） | `internal/rag/retrieve/` |

### M5：MCP + 联网检索（P2 · 能力扩展）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M5-1 | MCP 客户端 + 工具注册表（连接远程 MCP Server, tools/list 发现） | `internal/rag/mcp/` |
| M5-2 | LLM 参数提取（支持节点 param_prompt_template） | `internal/rag/mcp/extractor.go` |
| M5-3 | 工具执行 + 结果格式化进上下文 | `internal/rag/mcp/` |
| M5-4 | MCP_ONLY/MIXED Prompt 场景 | `prompt/planner.go` |
| M5-5 | You.com 联网检索通道（优先级最低） | `internal/rag/retrieve/` |

### M6：平台能力（P3 · 生产化）✅ 已完成并验收（2026-07-17）

| # | 任务 | 产出 |
|:--|------|------|
| M6-1 | 公平分布式限流（Lua 原子出队 + pub/sub + SSE REJECT 事件） | `internal/framework/ratelimit/` |
| M6-2 | 停止任务实装 + 幂等控制 | `pipeline/handler.go` |
| M6-3 | Trace 落库（节点耗时/TTFT → t_rag_trace_run/node）+ 查询 API 实装 | `internal/framework/trace/` |
| M6-4 | 审计日志（变更快照落 t_biz_change_log + 查询 API） | `internal/admin/` |
| M6-5 | 仪表板真实统计（overview/performance/trends） | `internal/admin/` |
| M6-6 | 用户管理 CRUD + 改密、示例问题 CRUD 实装 | `internal/admin/`, `internal/user/` |
| M6-7 | （可选）ES 关键词检索通道 | `internal/rag/retrieve/` |

### 降级与暂缓项

- RocketMQ（反馈/入库异步）→ 同步写库或 goroutine，必要时 Redis Stream
- PgVector / OSS / S3 → 仅 Milvus + 本地文件存储
- 效果评测 /rag/eval、GRAPH 图谱通道 → 暂缓

---

## Phase 1：骨架搭建 + 基础设施（2 周）

**目标**：项目可编译启动，可连接所有外部依赖，SSE 事件格式完整。

### 任务清单

| # | 任务 | 优先级 | 预估 | 产出文件 |
|:--|------|:------:|:----:|---------|
| 1.1 | 初始化 Go 项目 | P0 | 2h | `go.mod`, `Makefile`, `Dockerfile`, `.gitignore` |
| 1.2 | 目录结构搭建 | P0 | 2h | `cmd/`, `internal/`, `configs/`, `prompts/`, `migrations/` |
| 1.3 | Viper 配置加载 | P0 | 4h | `internal/config/`, `configs/config.yaml` |
| 1.4 | GORM + 数据库迁移 | P0 | 6h | `internal/repo/`, `migrations/*.sql` (8 张表) |
| 1.5 | go-redis 初始化 | P0 | 3h | `internal/infra/redis/` |
| 1.6 | Milvus Go SDK 连接 | P0 | 3h | `internal/rag/retrieve/vectorstore/milvus.go` |
| 1.7 | pgvector 连接 | P1 | 2h | `internal/rag/retrieve/vectorstore/pgvector.go` |
| 1.8 | Gin 路由 + 中间件链 | P0 | 4h | `cmd/server/main.go`, Gin 路由注册 |
| 1.9 | JWT 鉴权中间件 | P0 | 4h | `internal/framework/jwt/` |
| 1.10 | UserContext goroutine 上下文 | P0 | 2h | `internal/framework/userctx/` |
| 1.11 | Snowflake ID 生成 | P1 | 1h | `internal/framework/snowflake/` |
| 1.12 | SSE Emitter 封装 | P0 | 6h | `internal/framework/sse/` |
| 1.13 | 统一响应体 + 错误码 | P0 | 2h | `internal/framework/response/` |
| 1.14 | 日志初始化 (zap) | P0 | 2h | `internal/framework/log/` |
| 1.15 | Health 端点 | P1 | 1h | `GET /api/ragent/health` |

### Phase 1 验收标准

```bash
# 1. 编译通过
go build ./...

# 2. 启动服务不报错
go run cmd/server/main.go

# 3. Health 检查返回正确
curl http://localhost:9090/api/ragent/health
# {"code":"0","data":{"db":"OK","redis":"OK","milvus":"OK"}}

# 4. SSE 事件格式校验
curl -H "Authorization: Bearer <token>" \
  "http://localhost:9090/api/ragent/rag/v3/chat?question=test"
# event:meta
# data:{"conversationId":"...","taskId":"..."}
```

---

## Phase 2：核心 RAG 链路（3 周）⭐ 最重要

**目标**：GraphAgent Pipeline 跑通完整问答链路，前端 React 可直接对接。

### 任务清单

| # | 任务 | 优先级 | 预估 | 产出文件 |
|:--|------|:------:|:----:|---------|
| 2.1 | ModelRouter + CircuitBreaker | P0 | 8h | `internal/infra/llm/router.go`, `circuit.go` |
| 2.2 | ChatService 封装 | P0 | 6h | `internal/infra/llm/chat.go` (同步+流式) |
| 2.3 | Embedding 客户端 | P0 | 4h | `internal/infra/embedding/` |
| 2.4 | Rerank 客户端 | P1 | 4h | `internal/infra/rerank/` |
| 2.5 | Prompt 模板引擎 | P0 | 4h | `internal/rag/prompt/` (go:embed 14个.st) |
| 2.6 | 意图树加载器 | P0 | 8h | `internal/rag/intent/loader.go`, `tree_cache.go` |
| 2.7 | 意图分类器 | P0 | 6h | `internal/rag/intent/classifier.go` |
| 2.8 | 意图解析器 | P0 | 4h | `internal/rag/intent/resolver.go` |
| 2.9 | 查询改写 | P0 | 6h | `internal/rag/rewrite/` |
| 2.10 | 同义词归一化 | P1 | 3h | `internal/rag/rewrite/term_mapping.go` |
| 2.11 | SearchChannel 接口 + 上下文 | P0 | 3h | `internal/rag/retrieve/channel.go` |
| 2.12 | IntentDirectedSearchChannel | P0 | 6h | `internal/rag/retrieve/channel/intent_directed.go` |
| 2.13 | VectorGlobalSearchChannel | P0 | 6h | `internal/rag/retrieve/channel/vector_global.go` |
| 2.14 | KeywordSearchChannel | P1 | 6h | `internal/rag/retrieve/channel/keyword.go` |
| 2.15 | DedupPostProcessor | P0 | 3h | `internal/rag/retrieve/postprocessor/dedup.go` |
| 2.16 | FusionPostProcessor (RRF) | P0 | 4h | `internal/rag/retrieve/postprocessor/fusion.go` |
| 2.17 | RerankPostProcessor | P0 | 4h | `internal/rag/retrieve/postprocessor/rerank.go` |
| 2.18 | MultiChannelEngine | P0 | 8h | `internal/rag/retrieve/engine.go` |
| 2.19 | RetrievalEngine 顶层 | P0 | 6h | `internal/rag/retrieve/retrieval_engine.go` |
| 2.20 | MCP 参数提取器 | P0 | 4h | `internal/rag/mcp/extractor.go` |
| 2.21 | MCP 工具注册 + 执行 | P0 | 6h | `internal/rag/mcp/registry.go`, `executor.go` |
| 2.22 | MCP 结果格式化 | P0 | 4h | `internal/rag/mcp/formatter.go` |
| 2.23 | 对话记忆加载 | P0 | 6h | `internal/rag/memory/memory.go` |
| 2.24 | 对话记忆存储 (JDBCStore) | P0 | 4h | `internal/rag/memory/store.go` |
| 2.25 | Prompt 场景规划 | P0 | 4h | `internal/rag/prompt/planner.go` |
| 2.26 | 上下文格式化 | P0 | 4h | `internal/rag/prompt/formatter.go` |
| 2.27 | GraphAgent Pipeline 定义 | P0 | 8h | `internal/rag/pipeline/graph.go` |
| 2.28 | Pipeline 各节点函数 | P0 | 16h | `internal/rag/pipeline/nodes.go` |
| 2.29 | SSE Chat Handler | P0 | 4h | `internal/framework/sse/handler.go` |
| 2.30 | 停止任务 Handler | P1 | 2h | `POST /api/ragent/rag/v3/stop` |

### Phase 2 验收标准

```bash
# 1. 核心链路 curl 测试
curl -H "Authorization: Bearer <token>" \
  "http://localhost:9090/api/ragent/rag/v3/chat?question=OA系统数据安全规范是什么"
# → 返回正确的 SSE 流 (meta → message × N → finish → done)
# → 回答内容基于文档，非编造

# 2. 多轮对话
curl "...chat?question=那保险系统的呢&conversationId=<上轮ID>"
# → 历史上下文被带上，指代消解为"保险系统的数据安全规范"

# 3. 模型降级
# 手动关掉 qwen-plus 的 API Key
curl "...chat?question=测试"
# → 自动降级到 ollama qwen3:8b，回答正常

# 4. 检索覆盖
curl "...chat?question=打印机怎么换墨盒"
# → 意图分类命中 IT 支持
# → 返回相关的打印机换墨盒步骤

# 5. 前端对接验证
# 启动前端 React 项目，将 API_BASE_URL 指向 Go 服务
# → 问答界面正常工作，管理后台正常工作
```

---

## Phase 3：摘要 + 歧义引导 + 入库 Pipeline（2 周）

**目标**：长对话记忆不膨胀，歧义场景平滑引导，文档入库全链路可工作。

### 任务清单

| # | 任务 | 优先级 | 预估 | 产出文件 |
|:--|------|:------:|:----:|---------|
| 3.1 | SummaryService 摘要压缩 | P0 | 8h | `internal/rag/memory/summary.go` |
| 3.2 | 分布式摘要锁 | P0 | 3h | go-redis `SETNX` |
| 3.3 | 摘要异步触发 | P0 | 3h | goroutine pool |
| 3.4 | 歧义引导服务 | P0 | 8h | `internal/rag/guidance/` |
| 3.5 | 歧义 LLM 二次确认 | P0 | 4h | `internal/rag/guidance/llm_checker.go` |
| 3.6 | IngestionNode 接口 | P0 | 4h | `internal/ingestion/pipeline/node.go` |
| 3.7 | PipelineEngine | P0 | 8h | `internal/ingestion/pipeline/engine.go` |
| 3.8 | Fetcher 节点 | P0 | 4h | `internal/ingestion/nodes/fetcher.go` |
| 3.9 | Parser 节点（Tika/MinerU） | P0 | 8h | `internal/ingestion/nodes/parser.go` |
| 3.10 | Chunker 节点 | P0 | 6h | `internal/ingestion/nodes/chunker.go` |
| 3.11 | Enhancer 节点 | P1 | 6h | `internal/ingestion/nodes/enhancer.go` |
| 3.12 | Indexer 节点 | P0 | 6h | `internal/ingestion/nodes/indexer.go` |
| 3.13 | MinerU HTTP 客户端 | P0 | 4h | `internal/infra/mineru/` |
| 3.14 | RocketMQ 消费者 | P0 | 6h | `internal/ingestion/consumer.go` |

### Phase 3 验收标准

```bash
# 1. 摘要压缩
# 发起 >8 轮对话
# → 查 t_conversation_summary 表有摘要记录
# → 第 9 轮请求的 history[0] 包含摘要内容

# 2. 歧义引导
curl "...chat?question=数据安全规范"
# → 两个"数据安全"节点分数接近
# → 返回引导："您是指OA系统的数据安全，还是保险系统的数据安全？"

# 3. 文档入库
curl -X POST "http://localhost:9090/api/ragent/ingestion/tasks/upload" \
  -F "file=@test.pdf"
# → 等待入库完成
# → 在问答中能检索到 PDF 中的内容

# 4. RocketMQ 消费
# 上传文档 → MQ 消息 → 异步 pipeline 执行 → 入库完成
```

---

## Phase 4：限流 + Trace + 管理后台 API（2 周）

**目标**：高并发保护到位，全链路可观测，管理后台功能完整。

### 任务清单

| # | 任务 | 优先级 | 预估 | 产出文件 |
|:--|------|:------:|:----:|---------|
| 4.1 | 公平分布式限流器 | P0 | 8h | `internal/framework/ratelimit/limiter.go` |
| 4.2 | Lua 脚本加载 | P0 | 2h | `internal/framework/ratelimit/lua/` |
| 4.3 | Pub/Sub 通知 | P0 | 3h | `internal/framework/ratelimit/notifier.go` |
| 4.4 | SSE 排队状态推送 | P0 | 4h | ChatQueueLimiter 集成 |
| 4.5 | OpenTelemetry 初始化 | P0 | 4h | tRPC-Agent-Go 内置 |
| 4.6 | 自定义 Trace Span | P0 | 4h | 各节点加 Span |
| 4.7 | 管理后台：仪表板 API | P0 | 4h | `internal/admin/dashboard/` |
| 4.8 | 管理后台：知识库 CRUD API | P0 | 6h | `internal/admin/knowledge/` |
| 4.9 | 管理后台：文档管理 API | P0 | 4h | `internal/admin/datasets/` |
| 4.10 | 管理后台：意图树编辑 API | P0 | 6h | `internal/admin/intent_tree/` |
| 4.11 | 管理后台：入库监控 API | P0 | 4h | `internal/admin/ingestion/` |
| 4.12 | 管理后台：链路追踪 API | P0 | 6h | `internal/admin/traces/` |
| 4.13 | 管理后台：模型管理 API | P1 | 3h | `internal/admin/models/` |
| 4.14 | 管理后台：系统设置 API | P1 | 3h | `internal/admin/settings/` |
| 4.15 | 管理后台：用户管理 API | P1 | 3h | `internal/admin/users/` |
| 4.16 | 用户认证 API | P0 | 4h | `internal/user/` (登录/注册/登出) |
| 4.17 | 消息点赞/点踩 API | P1 | 2h | `internal/rag/message_feedback.go` |
| 4.18 | 会话管理 API | P0 | 4h | `internal/rag/conversation_handler.go` |

### Phase 4 验收标准

```bash
# 1. 分布式限流
# 并发 15 个请求
for i in {1..15}; do
  curl "...chat?question=test$i" &
done
# → 10 个执行中，5 个排队
# → 排队超时返回 SSE reject 事件

# 2. OpenTelemetry Trace
# 完成一次对话
# → Jaeger/Zipkin 能看到全部 9 个节点的 Span 耗时

# 3. 管理后台
# 前端访问管理后台页面
# → 仪表板统计数据正常
# → 知识库 CRUD 正常
# → 意图树编辑正常
# → Trace 详情树形视图正常
```

---

## Phase 5：测试 + 性能调优 + 文档（2 周）

**目标**：生产可用，有可量化的性能基线，文档齐全。

### 任务清单

| # | 任务 | 优先级 | 预估 | 产出文件 |
|:--|------|:------:|:----:|---------|
| 5.1 | 核心链路 E2E 测试 | P0 | 8h | `tests/e2e/chat_test.go` |
| 5.2 | 意图分类单元测试 | P0 | 4h | `internal/rag/intent/*_test.go` |
| 5.3 | 检索引擎单元测试 | P0 | 4h | `internal/rag/retrieve/*_test.go` |
| 5.4 | 记忆管理单元测试 | P0 | 4h | `internal/rag/memory/*_test.go` |
| 5.5 | 限流器单元测试 | P0 | 4h | `internal/framework/ratelimit/*_test.go` |
| 5.6 | 熔断器单元测试 | P0 | 4h | `internal/infra/llm/circuit_test.go` |
| 5.7 | 并发压测 | P0 | 6h | `tests/bench/`, 压测脚本 |
| 5.8 | P99 延迟优化 | P0 | 8h | goroutine pool 调优 |
| 5.9 | 内存优化 | P1 | 4h | pprof 分析 + 优化 |
| 5.10 | API 接口校验 | P1 | 2h | 所有接口端到端验证 |
| 5.11 | README.md | P0 | 2h | 项目概述 + 启动说明 |
| 5.12 | 架构设计文档 | P0 | 2h | `docs/architecture.md` |
| 5.13 | 开发任务文档 | P0 | 2h | `docs/development-tasks.md` |
| 5.14 | 交接文档 | P0 | 3h | `docs/handover.md` |

### Phase 5 验收标准

```bash
# 1. 测试通过
go test ./...
# → 所有测试绿

# 2. 性能基线
# 10 并发下：
#   P50 首包 < 5s
#   P99 首包 < 20s
#   无 goroutine 泄漏

# 3. 文档齐全
# README.md + architecture.md + development-tasks.md + handover.md
# 能根据文档从零搭建并跑通完整流程
```

---

## 风险与依赖

| 风险 | 影响 | 缓解方案 |
|------|------|---------|
| tRPC-Agent-Go API 变化 | Phase 2 核心链路 | 锁定 v1.10.0 版本，不追 latest |
| RocketMQ Go Client 不成熟 | Phase 3 入库 | 如遇阻塞问题，降级为 Redis Stream |
| MinerU API 限频 | Phase 3 解析 | 限流控制 + 队列缓冲 |
| 功能行为不一致 | Phase 2 回答质量 | 逐阶段端到端回归测试 |

## 技术债务记录

> 以下项目在 Phase 1-4 可先跳过，Phase 5 统一处理：

- ES 关键词检索在 keyword.type=none 时可跳过
- Rerank 如无 Cohere API Key 可先走 NoopReranker
- VLM 图生文（入库 Enhancer 节点）可先跳过
- OpenTelemetry 导出器在 P4 再对接
