# goRAGENT 项目交接文档

> 最后更新: 2026-07-17 | 版本: v1.0.0-dev

## 一、项目概要

**goRAGENT** 是 [Ragent AI](https://github.com/nageoffer/ragent)（Java 版企业级 Agentic RAG 平台）的 Go 语言重构版。前端 React 代码完全复用 Java 版，仅替换后端。

### 技术栈

| 层 | 技术 |
|:--|------|
| HTTP 框架 | Gin v1.10 |
| ORM | GORM + MySQL 驱动 |
| 缓存/Lock | go-redis v9 |
| 向量库 | Milvus Go SDK v2.4 |
| 鉴权 | golang-jwt v5 |
| LLM SDK | 自研 (OpenAI 兼容协议, OkHttp 风格) |
| 日志 | zap |
| 配置 | 环境变量驱动 (兼容 CarAgent 风格) |
| 可观测 | (预留 OpenTelemetry) |

### 规模

| 指标 | 数值 |
|------|:---:|
| Go 源文件 | 80+ 个 |
| 测试文件 | 4 个 test suite |
| Prompt 模板 | 14 个 .st |
| Docker 容器 | 6 个 (MySQL/Redis/Milvus/etcd/MinIO/Attu) |
| 路由 | 80+ 个 (全实装) |

---

## 二、快速启动

### 1. 基础设施

```bash
cd goRAGENT
docker compose -f docker/docker-compose.yml --project-name ragent up -d
```

| 服务 | 端口 | 账号/密码 |
|------|:---:|------|
| MySQL | 3307 | root/123456 |
| Redis | 6380 | 无密码 |
| Milvus | 19532 | — |
| Attu | 8083 | — |

### 2. 初始化数据库

```
docker exec -i ragent-mysql mysql -uroot -p123456 ragent < docker/init.sql
```

### 3. 启动后端

```bash
go run ./cmd/server/
# → http://localhost:9090
```

### 4. 启动前端

```bash
cd ../frontend
npm run dev
# → http://localhost:5175
```

---

## 三、架构

```
cmd/server/main.go              # 入口 + 依赖装配 (bootstrap)
pkg/                            # 可复用公共库 (零 internal 依赖)
  llm/                          # 模型路由 + 熔断 + ChatService
  embedding/                    # BGE-M3 HTTP 调用
  rerank/                       # BGE-M3 Rerank
  mineru/                       # MinerU HTTP API
  milvus/                       # Milvus gRPC 检索 (读写)
  prompt/                       # go:embed 模板引擎 + 14 个 .st
  sse/                          # SSE 事件协议 Emitter
  jwt/                          # JWT 鉴权
  snowflake/                    # 分布式 ID
  logx/                         # zap 日志
  response/                     # 统一响应体 Result
internal/
  model/                        # 领域模型 (纯 DO 结构体, 12 文件)
  handler/                      # HTTP Controller 层
    admin/                      # 管理后台 (知识库/文档/Chunk/意图/映射/入库/仪表板/用户/Trace/审计/示例问题)
    chat/                       # SSE 对话 Handler + StopTask
    auth/                       # 登录/注册/当前用户
  service/                      # 业务逻辑层
    rag/                        # RAG 核心: Pipeline/意图/改写/引导/检索/记忆/会话
    ingestion/                  # 入库 Pipeline: Fetcher→Parser→Chunker→Indexer
    mcp/                        # MCP 客户端 + 注册表 + 参数提取 + 执行器
  middleware/                    # HTTP 中间件 (JWT/限流/用户上下文)
  config/                       # 环境变量驱动配置
  bootstrap/                    # 依赖装配 (DB/Redis/Milvus/LLM 初始化)
docker/                         # Docker Compose + Nginx + init.sql
```

### 核心链路（8 阶段, 对齐 Java StreamChatPipeline）

```
HTTP GET /rag/v3/chat?question=xxx
  → JWT 鉴权 → SSE Emitter (handler 阻塞至流结束)
  → SimplePipeline.Execute():
      1. loadMemory     加载摘要(置顶注入)+最近8轮历史, user 消息落库
      2. rewrite        同义词归一化 → LLM 改写+子问题拆分 (失败回退规则拆分)
      3. resolveIntents 按子问题并行 LLM 意图分类 + capTotalIntents(保底1/上限3)
      4. guidance       歧义引导短路 (ratio≥0.8 直接判 / [0.65,0.8) LLM 二次确认 → 选项话术)
      5. systemOnly     SYSTEM 意图短路直答 (跳过检索, 节点模板可覆盖)
      6. retrieve       意图定向+全局向量检索 → Dedup/RRF/Rerank
      7. emptyRetrieval 空检索短路 ("未检索到与问题相关的文档内容。")
      8. streamResponse 组装 [system]+[history]+[evidence]+[question] → LLM 流式
  → 完成时: assistant 消息落库 → finish 带 messageId → 异步触发摘要压缩
  → SSE 事件: meta → message×N → finish → done
```

---

## 四、LLM 模型配置

从 `.env` 读取，支持 4 个 Provider：

```bash
LLM_PROVIDER=glm  # 默认主模型 (glm/openai/deepseek/qwen)

GLM_API_KEY=xxx
GLM_BASE_URL=https://open.bigmodel.cn/api/paas/v4
GLM_MODEL=glm-4-flash

OPENAI_API_KEY=xxx     # Mimo
OPENAI_BASE_URL=https://api.xiaomimimo.com/v1
OPENAI_MODEL=mimo-v2.5

DEEPSEEK_API_KEY=xxx
DEEPSEEK_MODEL=deepseek-v4-flash

QWEN_API_KEY=xxx
QWEN_MODEL=qwen-plus
```

降级链: `LLM_PROVIDER` → 其他已配置模型 → 全部失败则报错。

---

## 五、已完成功能清单

| 功能 | 状态 |
|------|:--:|
| 注册/登录/JWT 鉴权 | ✅ |
| SSE 流式问答 | ✅ |
| LLM 多模型路由 + 三态熔断 | ✅ |
| 多通道检索引擎 (去重/RRF/Rerank) | ✅ |
| Milvus 向量检索 (gRPC 读写) | ✅ |
| Prompt 模板引擎 (go:embed) | ✅ |
| 对话记忆 (DB 存储, 多轮上下文/消息落库/finish 带 messageId) | ✅ M1 |
| 会话管理 + 消息反馈 API (对齐前端 /conversations 契约) | ✅ M1 |
| 查询改写 + 同义词归一化 + 子问题拆分 (LLM+规则fallback) | ✅ M2 |
| 空检索短路 + 子问题并行意图分类 + 节点模板覆盖 | ✅ M2 |
| 关键词映射 CRUD (分页/清缓存, 对齐前端 /mappings 契约) | ✅ M2 |
| 对话摘要压缩 (9轮触发/Redis锁/摘要置顶注入) + LLM 会话标题 | ✅ M3 |
| 歧义引导 (分数比值+LLM二次确认→选项短路) + SYSTEM 意图直答 | ✅ M3 |
| 知识库 CRUD (Milvus Collection 生命周期) | ✅ M4 |
| 文档上传/列表/详情/预览/下载/启停/删除 | ✅ M4 |
| 入库 Pipeline (Fetcher→Parser(MinerU)→Chunker(混合切分)→Indexer(Embed+Milvus)) | ✅ M4 |
| Chunk 管理 CRUD + 启停用 | ✅ M4 |
| 入库任务监控 API | ✅ M4 |
| 检索元数据富化后处理器 (回表补 docId/docName) | ✅ M4 |
| MCP 客户端 + 工具注册表 + LLM 参数提取 + 执行器 | ✅ M5 |
| MCP_ONLY / MIXED 场景 Pipeline 分流 | ✅ M5 |
| You.com 联网检索通道 (priority=20 兜底) | ✅ M5 |
| 公平分布式限流 (Lua原子队列 + Redis pub/sub) | ✅ M6 |
| 停止任务实装 (taskID→cancel map) | ✅ M6 |
| Trace 落库 (t_rag_trace_run/node) + 查询 API | ✅ M6 |
| 审计日志 (t_biz_change_log) + 查询 API | ✅ M6 |
| 仪表板真实统计 (Overview/Performance/Trends) | ✅ M6 |
| 用户管理 CRUD + 改密 | ✅ M6 |
| 示例问题 CRUD (对齐前端 PageResult) | ✅ M6 |
| 管理后台 API (80+ 路由全部实装) | ✅ |
| Docker Compose (MySQL/Redis/Milvus) | ✅ |
| 工程结构重构 (pkg/model/service/handler/middleware 分层) | ✅ |
| 前端注册/登录页 | ✅ |
| 前端 Star/外链清理 | ✅ |
| Chat Pipeline SSE 崩溃修复 (handler 阻塞至流结束) | ✅ |
| 意图树加载 + LLM 分类 + 意图定向检索激活 | ✅ |
| 管理后台意图树 CRUD (对齐 Java 契约, 写后清缓存) | ✅ |

## 六、待完成项

| 迭代 | 内容 | 优先级 |
|------|------|:--:|
| ~~M1~~ | ~~对话记忆真实装 + 消息落库 + 会话管理/反馈 API~~ ✅ 2026-07-17 完成 | ~~P0~~ |
| ~~M2~~ | ~~查询改写 + 同义词 + 子问题并行意图分类 + 空检索短路~~ ✅ 2026-07-17 完成 | ~~P1~~ |
| ~~M3~~ | ~~摘要压缩 + 歧义引导 + SYSTEM 短路 + LLM 标题~~ ✅ 2026-07-17 完成 | ~~P2~~ |
| ~~M4~~ | ~~知识库/文档/Chunk 管理 + 入库 Pipeline (MinerU) + 元数据富化~~ ✅ 2026-07-17 完成 | ~~P2~~ |
| ~~M5~~ | ~~MCP 全套 + You.com 联网检索通道~~ ✅ 2026-07-17 完成 | ~~P2~~ |
| ~~M6~~ | ~~分布式限流 + Trace 落库 + 审计日志 + 仪表板真实统计 + 用户管理~~ ✅ 2026-07-17 完成 | ~~P3~~ |

**全部 6 个迭代已完成。** Java 版全量功能迁移完毕。

所有功能已全部实装，无空壳路由。

### dev 库注意事项（2026-07-17 已执行的结构变更）

- `t_intent_node` 已重建对齐 Java DDL（id 主键 + 数值 level/kind + sort_order），含 6 个测试节点（biz-hr/biz-it/sys-chat/oa-security/ins-security 等）
- `t_conversation_message` 已加 `vote` 列（反馈）
- `t_query_term_mapping` 已建（1 条测试映射：内网OA→OA系统）
- Milvus `ragent_knowledge` collection **尚不存在**（M4 入库后才有真实检索内容，当前问答走空检索短路）
- 测试账号：`intenttest / test123456`

### M3 新增环境变量（默认值）

```bash
MEMORY_HISTORY_KEEP_TURNS=8    # 历史加载轮数
MEMORY_TITLE_MAX_LENGTH=30     # 标题上限（LLM 生成, 失败截断）
MEMORY_SUMMARY_ENABLED=true    # 摘要压缩开关
MEMORY_SUMMARY_START_TURNS=9   # 触发轮数（须 > KEEP_TURNS）
MEMORY_SUMMARY_MAX_CHARS=200
GUIDANCE_ENABLED=true          # 歧义引导开关
GUIDANCE_AMBIGUITY_SCORE_RATIO=0.8
GUIDANCE_AMBIGUITY_MARGIN=0.15
GUIDANCE_MAX_OPTIONS=6
RAG_QUERY_REWRITE_ENABLED=true # 查询改写开关
```

---

## 七、项目端口

| 服务 | 端口 | 说明 |
|------|:---:|------|
| goRAGENT 后端 | 9090 | Go Gin |
| goRAGENT 前端 | 5175 | Vite Dev |
| goRAGENT MySQL | 3307 | Docker |
| goRAGENT Redis | 6380 | Docker |
| goRAGENT Milvus | 19532 | Docker |
| CarAgent 后端 | 8000 | Python |
| CarAgent 前端 | 5173 | Vite |
| CarAgent MySQL | 3306 | Docker |
| CarAgent Redis | 6379 | Docker |
| CarAgent Milvus | 19530 | Docker |
| Embedding HTTP | 19531 | BGE-M3 (共享) |

## 八、常用命令

```bash
# 重启后端
for pid in $(netstat -ano | grep ':9090 ' | awk '{print $5}'); do taskkill //F //PID $pid 2>/dev/null; done; sleep 1
cd goRAGENT && go run ./cmd/server/

# 重启前端
cd frontend && npx vite --host --port 5175

# 构建
cd goRAGENT && go build -o build/ragent-server.exe ./cmd/server/

# 测试
cd goRAGENT && go test ./... -count=1

# Docker
docker compose -f docker/docker-compose.yml --project-name ragent up -d
docker compose -f docker/docker-compose.yml --project-name ragent down
```
