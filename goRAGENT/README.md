# goRAGENT

Ragent AI 的 Go 语言重构版本，基于 [tRPC-Agent-Go](https://github.com/trpc-group/trpc-agent-go) 框架构建的企业级 Agentic RAG 平台。

## 项目概述

将原 Java 版 [Ragent AI](https://github.com/nageoffer/ragent) 后端完整重构为 Go，前端 React 保持不变。覆盖文档入库、意图识别、多通道检索、模型路由、MCP 工具调用、流式响应与全链路追踪的完整链路。

## 技术栈

| 领域 | 技术 | 对应 Java 版 |
|------|------|------------|
| 语言 | Go 1.23+ | Java 17 |
| HTTP 框架 | Gin | Spring Boot 3.5 |
| Agent 编排 | tRPC-Agent-Go GraphAgent | StreamChatPipeline（自研） |
| LLM SDK | tRPC-Agent-Go model/openai | infra-ai（自研） |
| ORM | GORM + gen | MyBatis-Plus |
| 缓存/锁/限流 | go-redis | Redisson |
| 向量库 | Milvus Go SDK + pgvector | Milvus Java SDK |
| 消息队列 | RocketMQ Go Client | RocketMQ Spring Starter |
| 鉴权 | golang-jwt | Sa-Token |
| 可观测性 | OpenTelemetry（框架内置） | AOP @RagTraceNode |
| 配置 | Viper | Spring @ConfigurationProperties |
| 日志 | zap | SLF4J/Logback |

## 快速开始

```bash
# 克隆
git clone https://github.com/nageoffer/ragent.git
cd ragent/goRAGENT

# 安装依赖
go mod download

# 配置
cp configs/config.example.yaml configs/config.yaml
# 编辑 config.yaml，填入数据库/Redis/LLM API Key

# 启动数据库迁移
go run cmd/server/main.go --migrate

# 启动服务
go run cmd/server/main.go

# 启动 MCP Server（可选）
go run cmd/mcp-server/main.go
```

## 项目结构

```
goRAGENT/
├── cmd/
│   ├── server/main.go         # 主服务入口
│   └── mcp-server/main.go     # MCP Server
├── internal/
│   ├── framework/             # 基础设施（SSE/JWT/限流/Snowflake）
│   ├── infra/                 # AI 层（LLM路由/Embedding/Rerank）
│   ├── rag/                   # RAG 核心（Pipeline/意图/检索/记忆/MCP）
│   ├── admin/                 # 管理后台 API
│   ├── user/                  # 用户认证
│   ├── ingestion/             # 文档入库 Pipeline
│   └── knowledge/             # 知识库 CRUD
├── prompts/                   # Prompt 模板（go:embed 编译进二进制）
├── migrations/                # PostgreSQL DDL
├── configs/                    # 配置文件
├── docs/                      # 文档
├── go.mod
├── go.sum
├── Makefile
└── Dockerfile
```

## 前端

前端代码保持不变，位于 `../frontend/`。Go 版的对外接口（URL、SSE 事件格式、JSON 字段名）与 Java 版完全一致，前端零改动即可对接。

## 与 Java 版的关系

| 模块 | Java | Go | 说明 |
|------|------|:--:|------|
| framework | Spring Boot 自研 | Gin + 标准库 | Go 精简 50%+ 代码量 |
| infra-ai | 自研 HTTP 客户端 | tRPC-Agent-Go model/openai | 框架内置 |
| rag pipeline | 硬编码 8 阶段 | GraphAgent StateGraph | 动态路由 |
| MCP | MCP Java SDK | 框架内置 MCP | 开箱即用 |
| 意图树 | 自研 | 自研（业务逻辑不变） | 1:1 复刻 |
| 检索引擎 | 自研 SPI | 自研（接口不变） | 1:1 复刻 |
| 记忆管理 | 自研 | 自研 + 框架 session | 混合方案 |
| 可观测性 | AOP @RagTraceNode | OpenTelemetry（框架内置） | 更强 |

## 相关文档

- [架构设计文档](docs/architecture.md)
- [开发任务文档](docs/development-tasks.md)
- [交接文档](docs/handover.md)

## License

Apache 2.0
