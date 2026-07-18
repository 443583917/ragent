# goRAGENT

基于 [tRPC-Agent-Go](https://github.com/trpc-group/trpc-agent-go) 框架构建的企业级 Agentic RAG 平台。

## 项目概述

企业级智能问答平台，覆盖文档入库、意图识别、多通道检索、模型路由、MCP 工具调用、流式响应与全链路追踪的完整链路。

## 技术栈

| 领域 | 技术 |
|------|------|
| 语言 | Go 1.23+ |
| HTTP 框架 | Gin |
| Agent 编排 | tRPC-Agent-Go GraphAgent |
| LLM SDK | tRPC-Agent-Go model/openai |
| ORM | GORM + gen |
| 缓存/锁/限流 | go-redis |
| 向量库 | Milvus Go SDK + pgvector |
| 消息队列 | RocketMQ Go Client |
| 鉴权 | golang-jwt |
| 可观测性 | OpenTelemetry（框架内置） |
| 配置 | Viper |
| 日志 | zap |

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

# 启动数据库初始化
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
│   ├── bootstrap/             # 依赖装配
│   ├── config/                # 配置加载
│   ├── router/                # 路由注册
│   ├── handler/               # HTTP 层（薄 Controller）
│   ├── service/               # 业务逻辑层
│   │   ├── rag/               # RAG 核心（Pipeline/意图/检索/记忆/MCP）
│   │   ├── admin/             # 管理后台服务
│   │   ├── auth/              # 用户认证
│   │   ├── ingestion/         # 文档入库 Pipeline
│   │   └── mcp/               # MCP 工具集
│   ├── repository/            # 数据访问接口
│   │   └── mysql/             # GORM 实现
│   ├── model/                 # 领域模型（DO/DTO/VO/常量）
│   └── middleware/            # HTTP 中间件
├── pkg/                       # 可复用公共库
│   ├── llm/                   # LLM 路由/熔断/ChatService
│   ├── embedding/             # Embedding 客户端
│   ├── rerank/                # Rerank 客户端
│   ├── milvus/                # 向量库客户端
│   ├── prompt/                # Prompt 模板引擎（go:embed）
│   ├── sse/                   # SSE 事件协议
│   └── jwt/ / snowflake/ / logx/ / errs/ / response/ / mineru/
├── prompts/                   # Prompt 模板（go:embed 编译进二进制）
├── migrations/                # 数据库 DDL
├── configs/                   # 配置文件
├── docs/                      # 文档
├── go.mod
├── go.sum
├── Makefile
└── Dockerfile
```

## 前端

前端代码位于 `../frontend/`，基于 React + Vite 构建。

## 相关文档

- [架构设计文档](docs/architecture.md)
- [开发任务文档](docs/development-tasks.md)
- [交接文档](docs/handover.md)

## License

Apache 2.0
