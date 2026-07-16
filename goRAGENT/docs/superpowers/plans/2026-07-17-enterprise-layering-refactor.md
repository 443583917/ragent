# goRAGENT 企业级分层重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 引入 repository 层与全链路接口化依赖注入，使 handler 只做 HTTP、service 只做业务、repository 只做数据访问，消灭 21 处散落的 `*gorm.DB` 直接访问。

**Architecture:** 标准四层（handler → service → repository → model）+ pkg 公共库 + bootstrap 装配。service/repository 全部以 interface 暴露、构造函数注入；DTO/VO 收敛到 model 层；分页/错误渲染抽公共函数；魔法值常量化。

**Tech Stack:** Go 1.2x / Gin / GORM(MySQL) / zap / go-redis / Milvus SDK。测试：标准库 + testify（已有）+ 手写 mock（禁止 mock 框架）+ glebarez/sqlite（repo 层内存库测试，纯 Go 无 CGO）。

## Global Constraints（每个任务都隐含这些约束）

1. **API 100% 兼容**：所有路由路径、响应 JSON 结构、分页参数风格（`page/pageSize` 与 `current/size` 两种风格按现有端点保持不变）、错误码字符串必须与重构前完全一致。行为规范以现有代码为准 —— 动手前必须先读被迁移的原文件。
2. **命名**：HTTP 层目录保持 `handler`（不是 controller）。包名小写单数；常量 CamelCase（非全大写）；文件 snake_case。
3. **repository 规则**：所有方法第一参数 `context.Context`；只做数据访问，零业务逻辑；接口定义在 `internal/repository/`，GORM 实现在 `internal/repository/mysql/`。
4. **service 规则**：以 interface 暴露（接口和实现同文件，实现结构体不导出）；构造函数 `NewXxx(...)` 返回接口类型；只依赖 repository 接口 + pkg + model，禁止 import gin 和 gorm。
5. **handler 规则**：只做参数绑定/校验 → 调 service → 渲染响应；禁止 import gorm。
6. **错误处理**：service/repository 返回 `pkg/errs` 包装的错误（`errs.WrapServer(err, "查询知识库列表失败")` 等）；禁止 `_` 忽略 error（errcheck check-blank 已开）；handler 用 `httpx.Error(c, err)` 统一渲染。
7. **日志**：只用 zap 结构化日志（`zap.L()` 或注入 logger），禁止 fmt.Print / log 标准库。
8. **依赖方向**：pkg 不 import internal；model 不 import 项目内任何包（可 import 标准库）。
9. **密码**：保持 MD5 兼容存量数据（本次不换 bcrypt），但哈希逻辑从 model 层移出、以 `PasswordHasher` 接口抽象。
10. **每个任务结束**：`go build ./...` + `go vet ./...` + `go test ./...` 全绿后才 commit；commit message 遵循 `refactor(scope): 描述` 规范。
11. **测试**：手写 mock（interface + struct），表格驱动，命名 `Test{类型}_{方法}_{场景}`。
12. 所有命令在 `goRAGENT/` 目录下执行（go.mod 所在目录）。

---

## 目标目录结构（重构完成态）

```
goRAGENT/
├── cmd/server/main.go              # 仅入口：加载配置 → bootstrap → 启动（≤60 行）
├── internal/
│   ├── bootstrap/                  # 依赖装配（唯一知道所有具体实现的地方）
│   │   ├── bootstrap.go            # App 结构：装配 infra→repo→service→handler→router
│   │   └── probe.go                # 启动自检（原 cmd/server/init.go 的 InitDB 等）
│   ├── config/config.go            # 纯配置加载（删除 SetDBGorm/GetDB 服务定位器）
│   ├── router/router.go            # 模块化路由组装（不再接触 *gorm.DB）
│   ├── middleware/                 # auth.go / ratelimit.go / userctx.go
│   ├── handler/                    # HTTP 层（薄）
│   │   ├── httpx/httpx.go          # 分页参数解析 + 统一渲染 helper
│   │   ├── admin/                  # 12 个文件，只剩 bind→service→render
│   │   ├── auth/handler.go
│   │   ├── chat/handler.go
│   │   └── session/handler.go      # 从 service/rag/session_service.go 拆出的 HTTP 部分
│   ├── service/
│   │   ├── admin/                  # 后台业务（全部接口化）
│   │   ├── auth/                   # 登录注册 + PasswordHasher
│   │   ├── rag/                    # RAG 编排（DB 访问全部改走 repository）
│   │   ├── ingestion/              # 入库流水线（同上）
│   │   └── mcp/
│   ├── repository/                 # 数据访问接口（按领域一文件一接口组）
│   │   ├── repository.go           # Repositories 聚合结构
│   │   ├── user.go / knowledge.go / conversation.go / intent.go / mapping.go
│   │   ├── trace.go / audit.go / sample_question.go / dashboard.go
│   │   └── mysql/                  # GORM 实现 + 公共 scope（软删除/分页）
│   └── model/                      # DO + DTO/VO + 常量（零内部依赖）
│       ├── *.go                    # 既有 DO
│       ├── page.go                 # PageQuery / PageResult
│       ├── consts.go               # 状态/角色/维度等常量
│       └── *_dto.go                # 从 handler 迁入的 req/VO 结构体
└── pkg/                            # errs / response / logx / jwt / sse / llm / ...
```

### 分层职责

| 层 | 职责 | 可依赖 |
|---|---|---|
| handler | HTTP 绑定/校验/渲染 | service 接口, model, httpx, pkg/response |
| service | 业务编排、事务边界、跨资源协调（如 Milvus+DB） | repository 接口, model, pkg |
| repository | 单表/聚合数据访问，context 透传 | model, gorm(仅 mysql/ 子包) |
| model | DO/DTO/VO/常量，纯数据 | 标准库 |
| middleware | HTTP 拦截 | pkg |
| bootstrap | 具体实现装配 | 所有层 |
| pkg | 可复用库 | 外部 SDK |

---

## 核心接口定义（后续任务统一引用，签名必须逐字一致）

### internal/repository/repository.go

```go
package repository

// Repositories 聚合全部数据访问接口，由 bootstrap 装配、按需注入各 service。
type Repositories struct {
	User           UserRepository
	KnowledgeBase  KnowledgeBaseRepository
	Document       DocumentRepository
	Chunk          ChunkRepository
	IngestionTask  IngestionTaskRepository
	Conversation   ConversationRepository
	Message        MessageRepository
	Summary        SummaryRepository
	IntentNode     IntentNodeRepository
	TermMapping    TermMappingRepository
	Trace          TraceRepository
	AuditLog       AuditLogRepository
	SampleQuestion SampleQuestionRepository
	Dashboard      DashboardRepository
}
```

### internal/repository/user.go

```go
package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// UserRepository 用户表数据访问。
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*model.UserDO, error)
	FindByUsername(ctx context.Context, username string) (*model.UserDO, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error)
	Create(ctx context.Context, u *model.UserDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}
```

### internal/repository/knowledge.go

```go
package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// KnowledgeBaseRepository 知识库表数据访问。
type KnowledgeBaseRepository interface {
	List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.KnowledgeBaseDO, error)
	Create(ctx context.Context, kb *model.KnowledgeBaseDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}

// DocumentRepository 文档表数据访问。
type DocumentRepository interface {
	ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.DocumentDO, int64, error)
	Search(ctx context.Context, keyword string, q model.PageQuery) ([]model.DocumentDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.DocumentDO, error)
	FindByIDs(ctx context.Context, ids []string) ([]model.DocumentDO, error)
	Create(ctx context.Context, d *model.DocumentDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}

// ChunkRepository 分块表数据访问。
type ChunkRepository interface {
	ListByKB(ctx context.Context, kbID string, q model.PageQuery) ([]model.ChunkDO, int64, error)
	ListByDoc(ctx context.Context, docID string, q model.PageQuery) ([]model.ChunkDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.ChunkDO, error)
	FindByIDs(ctx context.Context, ids []string) ([]model.ChunkDO, error)
	BatchCreate(ctx context.Context, chunks []model.ChunkDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDeleteByDoc(ctx context.Context, docID string) error
}

// IngestionTaskRepository 入库任务表数据访问。
type IngestionTaskRepository interface {
	List(ctx context.Context, q model.PageQuery) ([]model.IngestionTaskDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.IngestionTaskDO, error)
	Create(ctx context.Context, t *model.IngestionTaskDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
}
```

### internal/repository/conversation.go

```go
package repository

import (
	"context"

	"goRAGENT/internal/model"
)

// ConversationRepository 会话表数据访问。
type ConversationRepository interface {
	ListByUser(ctx context.Context, userID string, limit int) ([]model.ConversationDO, error)
	Exists(ctx context.Context, id string) (bool, error)
	Create(ctx context.Context, c *model.ConversationDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}

// MessageRepository 会话消息表数据访问。
type MessageRepository interface {
	ListByConversation(ctx context.Context, convID string) ([]model.ConversationMessageDO, error)
	ListRecent(ctx context.Context, convID string, limit int) ([]model.ConversationMessageDO, error)
	ListRange(ctx context.Context, convID string, afterID, beforeID int64) ([]model.ConversationMessageDO, error)
	CountUserMessages(ctx context.Context, convID string) (int64, error)
	Create(ctx context.Context, m *model.ConversationMessageDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
}

// SummaryRepository 会话摘要表数据访问。
type SummaryRepository interface {
	Latest(ctx context.Context, convID string) (*model.ConversationSummaryDO, error)
	Create(ctx context.Context, s *model.ConversationSummaryDO) error
}
```

### internal/repository/intent.go / mapping.go / trace.go / audit.go / sample_question.go

```go
// intent.go
type IntentNodeRepository interface {
	ListActive(ctx context.Context) ([]model.IntentNodeDO, error) // deleted=0 AND enabled=1
	ListAll(ctx context.Context) ([]model.IntentNodeDO, error)    // deleted=0
	Create(ctx context.Context, n *model.IntentNodeDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
	BatchUpdateFields(ctx context.Context, ids []string, updates map[string]any) error
}

// mapping.go
type TermMappingRepository interface {
	ListEnabled(ctx context.Context) ([]model.TermMappingDO, error)
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.TermMappingDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.TermMappingDO, error)
	Create(ctx context.Context, m *model.TermMappingDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}

// trace.go — TraceRunFilter 定义在 model/trace.go（字段对照现有 listTraceRunsReal 的查询条件）
type TraceRepository interface {
	CreateRun(ctx context.Context, run *model.TraceRunDO) error
	UpdateRunFieldsByTaskID(ctx context.Context, taskID string, updates map[string]any) error
	ListRuns(ctx context.Context, q model.PageQuery, f model.TraceRunFilter) ([]model.TraceRunDO, int64, error)
	FindRun(ctx context.Context, runID string) (*model.TraceRunDO, error)
	ListNodes(ctx context.Context, runID string) ([]model.TraceNodeDO, error)
}

// audit.go — AuditLogFilter 同理定义在 model/audit.go
type AuditLogRepository interface {
	List(ctx context.Context, q model.PageQuery, f model.AuditLogFilter) ([]model.BizChangeLogDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.BizChangeLogDO, error)
	Create(ctx context.Context, l *model.BizChangeLogDO) error
}

// sample_question.go
type SampleQuestionRepository interface {
	List(ctx context.Context, q model.PageQuery, keyword string) ([]model.SampleQuestionDO, int64, error)
	ListPublic(ctx context.Context, limit int) ([]model.SampleQuestionDO, error) // deleted=0 AND enabled=1
	Create(ctx context.Context, s *model.SampleQuestionDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
	SoftDelete(ctx context.Context, id string) error
}
```

### internal/repository/dashboard.go（读模型聚合，语义以现有 dashboard.go 为准）

```go
type DashboardRepository interface {
	Stats(ctx context.Context) (*model.DashboardStats, error)
	Overview(ctx context.Context, since time.Time) (*model.DashboardOverview, error)
	Performance(ctx context.Context, since time.Time) (*model.DashboardPerformance, error)
	// TrendCounts 按时间桶统计指定指标数量；metric 取值见 model/consts.go TrendMetric* 常量
	TrendCounts(ctx context.Context, metric string, buckets []model.TimeBucket) ([]int64, error)
}
```

（`DashboardStats/Overview/Performance/TimeBucket` 结构体在 Task 1 中依据 `internal/handler/admin/dashboard.go` 现有响应字段定义到 `internal/model/dashboard_dto.go`。）

### internal/model/page.go（Task 1 创建，全文）

```go
package model

// 分页默认值与上限
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 200
)

// PageQuery 统一分页入参（handler 层负责从两种参数风格解析）。
type PageQuery struct {
	Page int // 从 1 开始
	Size int
}

// Normalize 纠正非法分页参数，返回可安全用于 SQL 的值。
func (q PageQuery) Normalize() PageQuery {
	if q.Page < 1 {
		q.Page = DefaultPage
	}
	if q.Size < 1 {
		q.Size = DefaultPageSize
	}
	if q.Size > MaxPageSize {
		q.Size = MaxPageSize
	}
	return q
}

// Offset 计算 SQL OFFSET。
func (q PageQuery) Offset() int { return (q.Page - 1) * q.Size }

// PageResult 对齐前端 PageResult<T> 的分页响应体（current/size 风格端点使用）。
type PageResult struct {
	Records any   `json:"records"`
	Total   int64 `json:"total"`
	Size    int   `json:"size"`
	Current int   `json:"current"`
	Pages   int64 `json:"pages"`
}

// NewPageResult 构造分页响应；pages 向上取整。
func NewPageResult(records any, total int64, q PageQuery) PageResult {
	pages := total / int64(q.Size)
	if total%int64(q.Size) != 0 {
		pages++
	}
	return PageResult{Records: records, Total: total, Size: q.Size, Current: q.Page, Pages: pages}
}
```

> ⚠️ 注意：现有 `query_term_mapping.go` 里已有 `buildPageResult`，Task 1 时先读它，若字段名（records/total/size/current/pages）与上面不一致，**以现有代码为准修改上面的定义**，并删除旧的重复实现。`total/rows` 风格端点（如 knowledge_base 列表）保持 `gin.H{"total": total, "rows": vos}` 原样，不迁移到 PageResult。

### internal/handler/httpx/httpx.go（Task 5 创建，全文）

```go
// Package httpx 提供 handler 层公共的参数解析与响应渲染函数。
package httpx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"goRAGENT/internal/model"
	"goRAGENT/pkg/response"
)

// PageFromQuery 解析 page/pageSize 风格分页参数（非法值回退默认）。
func PageFromQuery(c *gin.Context) model.PageQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(model.DefaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", strconv.Itoa(model.DefaultPageSize)))
	return model.PageQuery{Page: page, Size: size}.Normalize()
}

// PageFromCurrentSize 解析 current/size 风格分页参数（对齐前端 PageResult<T> 端点）。
func PageFromCurrentSize(c *gin.Context) model.PageQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("current", strconv.Itoa(model.DefaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(model.DefaultPageSize)))
	return model.PageQuery{Page: page, Size: size}.Normalize()
}

// OK 渲染成功响应。
func OK(c *gin.Context, data any) { c.JSON(http.StatusOK, response.Success(data)) }

// OKEmpty 渲染无数据成功响应。
func OKEmpty(c *gin.Context) { c.JSON(http.StatusOK, response.SuccessOK()) }

// Error 将 service 层错误渲染为统一响应（HTTP 200 + 业务错误码，保持现有前端契约）。
func Error(c *gin.Context, err error) { c.JSON(http.StatusOK, response.FromError(err)) }

// BadRequest 参数绑定失败的快捷渲染。
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusOK, response.Failure(response.CodeParamError, message))
}
```

> ⚠️ 兼容性注意：auth 相关端点现在返回非 200 的 HTTP 状态码（401/409/500），改造 auth handler 时保留原 HTTP 状态码行为，不套用 `httpx.Error`（它固定 200）。为此 auth handler 可继续用 `c.JSON(http.StatusUnauthorized, response.FromError(err))` 形式。

### 示范：knowledge 域端到端参考实现（其余域照此模式）

**internal/repository/mysql/mysql.go**：

```go
// Package mysql 提供 repository 接口的 GORM/MySQL 实现。
package mysql

import (
	"gorm.io/gorm"

	"goRAGENT/internal/repository"
)

// New 装配全部 MySQL repository 实现。
func New(db *gorm.DB) repository.Repositories {
	return repository.Repositories{
		User:           NewUserRepo(db),
		KnowledgeBase:  NewKnowledgeBaseRepo(db),
		Document:       NewDocumentRepo(db),
		Chunk:          NewChunkRepo(db),
		IngestionTask:  NewIngestionTaskRepo(db),
		Conversation:   NewConversationRepo(db),
		Message:        NewMessageRepo(db),
		Summary:        NewSummaryRepo(db),
		IntentNode:     NewIntentNodeRepo(db),
		TermMapping:    NewTermMappingRepo(db),
		Trace:          NewTraceRepo(db),
		AuditLog:       NewAuditLogRepo(db),
		SampleQuestion: NewSampleQuestionRepo(db),
		Dashboard:      NewDashboardRepo(db),
	}
}

// notDeleted 软删除过滤公共 Scope。
func notDeleted(db *gorm.DB) *gorm.DB { return db.Where("deleted = 0") }
```

**internal/repository/mysql/knowledge_base_repo.go**（完整参考）：

```go
package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// knowledgeBaseRepo KnowledgeBaseRepository 的 GORM 实现。
type knowledgeBaseRepo struct{ db *gorm.DB }

// NewKnowledgeBaseRepo 创建知识库 repository。
func NewKnowledgeBaseRepo(db *gorm.DB) repository.KnowledgeBaseRepository {
	return &knowledgeBaseRepo{db: db}
}

func (r *knowledgeBaseRepo) List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).Scopes(notDeleted)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count knowledge bases: %w", err)
	}
	var dos []model.KnowledgeBaseDO
	if err := tx.Order("create_time DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list knowledge bases: %w", err)
	}
	return dos, total, nil
}

func (r *knowledgeBaseRepo) FindByID(ctx context.Context, id string) (*model.KnowledgeBaseDO, error) {
	var do model.KnowledgeBaseDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find knowledge base id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *knowledgeBaseRepo) Create(ctx context.Context, kb *model.KnowledgeBaseDO) error {
	if err := r.db.WithContext(ctx).Create(kb).Error; err != nil {
		return fmt.Errorf("create knowledge base: %w", err)
	}
	return nil
}

func (r *knowledgeBaseRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update knowledge base id=%s: %w", id, err)
	}
	return nil
}

func (r *knowledgeBaseRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete knowledge base id=%s: %w", id, err)
	}
	return nil
}
```

**internal/service/admin/knowledge_service.go**（完整参考；注意 `errors.Is(err, gorm.ErrRecordNotFound)` 判断不可用——service 不 import gorm，因此 repo 的 FindByID 错误在 service 侧统一转 NotFound）：

```go
// Package admin 提供管理后台业务服务。
package admin

import (
	"context"

	"go.uber.org/zap"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/snowflake"
)

// VectorStore service 层所需的向量库能力抽象（由 pkg/milvus.MilvusStore 满足）。
type VectorStore interface {
	CreateCollection(ctx context.Context, name string, dim int) error
	DropCollection(ctx context.Context, name string) error
}

// KnowledgeBaseService 知识库业务。
type KnowledgeBaseService interface {
	List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseVO, int64, error)
	Create(ctx context.Context, req model.KnowledgeBaseCreateReq) (*model.KnowledgeBaseVO, error)
	Get(ctx context.Context, id string) (*model.KnowledgeBaseVO, error)
	Update(ctx context.Context, id string, req model.KnowledgeBaseUpdateReq) error
	Delete(ctx context.Context, id string) error
}

type knowledgeBaseService struct {
	repo   repository.KnowledgeBaseRepository
	vector VectorStore // 可为 nil（未配置 Milvus）
	logger *zap.Logger
}

// NewKnowledgeBaseService 创建知识库业务服务。
func NewKnowledgeBaseService(repo repository.KnowledgeBaseRepository, vector VectorStore, logger *zap.Logger) KnowledgeBaseService {
	return &knowledgeBaseService{repo: repo, vector: vector, logger: logger}
}

func (s *knowledgeBaseService) List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseVO, int64, error) {
	dos, total, err := s.repo.List(ctx, q)
	if err != nil {
		s.logger.Error("查询知识库列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.KnowledgeBaseVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.KnowledgeBaseDOToVO(d))
	}
	return vos, total, nil
}

func (s *knowledgeBaseService) Create(ctx context.Context, req model.KnowledgeBaseCreateReq) (*model.KnowledgeBaseVO, error) {
	id := snowflake.NextID()
	collectionName := model.KBCollectionPrefix + id
	if s.vector != nil {
		if err := s.vector.CreateCollection(ctx, collectionName, model.DefaultVectorDimension); err != nil {
			s.logger.Error("创建 Milvus Collection 失败", zap.Error(err))
			return nil, errs.WrapBusiness(err, "创建向量集合失败")
		}
	}
	do := model.KnowledgeBaseDO{
		ID: id, Name: req.Name, Description: req.Description,
		CollectionName: collectionName, Dimension: model.DefaultVectorDimension,
	}
	if err := s.repo.Create(ctx, &do); err != nil {
		s.logger.Error("创建知识库失败", zap.Error(err))
		return nil, errs.WrapBusiness(err, "创建失败")
	}
	vo := model.KnowledgeBaseDOToVO(do)
	return &vo, nil
}

func (s *knowledgeBaseService) Get(ctx context.Context, id string) (*model.KnowledgeBaseVO, error) {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errs.Business("知识库不存在") // 见下方错误码兼容性说明：必须 B000002
	}
	vo := model.KnowledgeBaseDOToVO(*do)
	return &vo, nil
}

func (s *knowledgeBaseService) Update(ctx context.Context, id string, req model.KnowledgeBaseUpdateReq) error {
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		s.logger.Error("更新知识库失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	return nil
}

func (s *knowledgeBaseService) Delete(ctx context.Context, id string) error {
	do, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return errs.Business("知识库不存在")
	}
	if s.vector != nil && do.CollectionName != "" {
		if err := s.vector.DropCollection(ctx, do.CollectionName); err != nil {
			s.logger.Warn("删除 Milvus Collection 失败（继续删除记录）", zap.Error(err))
		}
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		s.logger.Error("删除知识库失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	return nil
}
```

> ⚠️ **错误码兼容性**：现有前端收到的"不存在/失败"类错误码是 `B000002`（CodeBusinessError）。因此 service 返回"资源不存在"时**必须继续用 `errs.Business("知识库不存在")`**（B000002），不要用新的 `errs.NotFound`（B000003）——B000003 本次不下发，仅保留给未来新接口。

**internal/handler/admin/knowledge_base.go 重构后**（完整参考）：

```go
package admin

import (
	"github.com/gin-gonic/gin"

	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listKnowledgeBases(c *gin.Context) {
	vos, total, err := h.svc.KnowledgeBase.List(c.Request.Context(), httpx.PageFromQuery(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) createKnowledgeBase(c *gin.Context) {
	var req model.KnowledgeBaseCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name 不能为空")
		return
	}
	vo, err := h.svc.KnowledgeBase.Create(c.Request.Context(), req)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}
// get/update/delete 同模式：解析参数 → svc 调用 → httpx 渲染
```

`Handler` 结构体重构为只持有服务集合：

```go
// Services admin handler 依赖的全部业务服务（bootstrap 装配）。
type Services struct {
	Dashboard      DashboardService
	KnowledgeBase  KnowledgeBaseService
	Document       DocumentService
	Chunk          ChunkService
	IngestionTask  IngestionTaskService
	Intent         IntentService
	Mapping        MappingService
	User           UserService
	Trace          TraceService
	Audit          AuditService
	SampleQuestion SampleQuestionService
}

type Handler struct{ svc svcadmin.Services } // 注意包名冲突：import svcadmin "goRAGENT/internal/service/admin"
```

（`Services` 定义放在 `internal/service/admin/services.go`；`DB()`、全部 `SetXxx` setter 删除。）

---

## 任务列表

### Task 1: model 层整备（分页/常量/DTO 骨架/去重）

**Files:**
- Create: `internal/model/page.go`（上文全文）
- Create: `internal/model/page_test.go`
- Create: `internal/model/consts.go`
- Modify: `internal/model/user.go`（直接删除 `MD5Hash`，引用方 `internal/handler/auth/handler.go`、`internal/handler/admin/user_mgmt.go` 各自临时内联私有函数 `md5Hash`（函数体照抄原 `model.MD5Hash`），Task 3/4 再收敛到 PasswordHasher）
- Modify: `internal/service/rag/term_mapping.go`（删除重复定义的 `TermMappingDO`，改用 `model.TermMappingDO`；先读 `internal/model/mapping.go` 比对字段确认一致）
- Modify: `internal/handler/admin/query_term_mapping.go`（`buildPageResult` 替换为 `model.NewPageResult`，删除旧实现；同风格的 trace.go / sample_question.go 一并替换）

**internal/model/consts.go 全文：**

```go
package model

// 向量库相关常量
const (
	// DefaultVectorDimension 默认向量维度（BGE-M3 / OpenAI ada 兼容维度）
	DefaultVectorDimension = 1536
	// KBCollectionPrefix 知识库对应 Milvus Collection 名前缀
	KBCollectionPrefix = "kb_"
)

// TraceRun 状态（对照 t_rag_trace_run.status 既有取值）
const (
	TraceStatusRunning   = "RUNNING"
	TraceStatusSuccess   = "SUCCESS"
	TraceStatusFailed    = "FAILED"
	TraceStatusCancelled = "CANCELLED"
	TraceStatusEmpty     = "EMPTY"
)

// 用户角色
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// 消息角色（t_conversation_message.role）
const (
	MsgRoleUser      = "user"
	MsgRoleAssistant = "assistant"
)

// DocumentPreviewMaxRunes 文档预览截断长度（原 document.go 硬编码 5000）
const DocumentPreviewMaxRunes = 5000

// 软删除标记
const (
	NotDeleted = 0
	Deleted    = 1
)

// 启用标记
const (
	Disabled = 0
	Enabled  = 1
)
```

> ⚠️ 动手前 grep 现有代码里的状态字符串（`"RUNNING"`、`"CANCELLED"`、`"EMPTY"`、`'user'` 等），确认常量取值与现状逐字一致；发现本清单遗漏的状态值就补进 consts.go。

**Steps:**
- [ ] **Step 1**: 读 `internal/handler/admin/query_term_mapping.go` 的 `buildPageResult` 与测试 `query_term_mapping_test.go`，确认 PageResult 字段名；写 `internal/model/page_test.go`（表格驱动：Normalize 越界回退、Offset 计算、NewPageResult pages 取整——含 total=0、整除、不整除三例）
- [ ] **Step 2**: `go test ./internal/model/` 确认失败（包不存在函数）
- [ ] **Step 3**: 创建 page.go / consts.go；替换三处 buildPageResult 同类实现；删除重复 TermMappingDO；处理 MD5Hash 迁移（临时内联）
- [ ] **Step 4**: `go build ./... && go test ./...` 全绿（原 query_term_mapping_test.go 若测 buildPageResult 则迁移断言到 page_test.go）
- [ ] **Step 5**: `git commit -m "refactor(model): 统一分页类型与业务常量，消除重复 DO 定义"`

### Task 2: repository 接口 + MySQL 实现（全部 14 个）

**Files:**
- Create: `internal/repository/{repository,user,knowledge,conversation,intent,mapping,trace,audit,sample_question,dashboard}.go`（接口代码见上文「核心接口定义」，逐字采用）
- Create: `internal/repository/mysql/mysql.go` + 每接口一个 `*_repo.go` 实现文件
- Create: `internal/repository/mysql/knowledge_base_repo_test.go`、`user_repo_test.go`（glebarez/sqlite 内存库：建表 → CRUD → 断言；仅这两个代表性测试）
- Modify: `internal/model/trace.go`（增加 `TraceRunFilter`）、`internal/model/audit.go`（增加 `AuditLogFilter`）、Create: `internal/model/dashboard_dto.go`

**实现依据（必须先读）：** 每个 repo 方法的 WHERE/ORDER/字段语义逐一对照现有 handler/service 代码：`internal/handler/admin/*.go`（各域 CRUD）、`internal/service/rag/{memory_service,session_service,intent_loader,term_mapping,metadata_enrich}.go`、`internal/service/ingestion/{engine,indexer}.go`、`internal/handler/chat/handler.go`。Dashboard 聚合的 SQL 语义照抄 `dashboard.go`（保留 `.Table("t_conversation")` 写法可改为 Model 引用，结果必须等价；TrendCounts 按桶循环 Count 的现状照搬进 repo——性能优化不在本次范围）。

**Steps:**
- [ ] **Step 1**: `go get github.com/glebarez/sqlite`（纯 Go，无 CGO）
- [ ] **Step 2**: 写接口文件（逐字采用上文定义）+ model 侧 Filter/DTO 结构
- [ ] **Step 3**: 写 `knowledge_base_repo_test.go`（内存库 AutoMigrate KnowledgeBaseDO → Create → List 分页 → UpdateFields → SoftDelete 后 FindByID 报错）与 `user_repo_test.go`（Create → FindByUsername → ExistsByUsername → List keyword 过滤）；跑测试确认失败
- [ ] **Step 4**: 实现全部 `*_repo.go`（模式照抄上文 knowledge_base_repo.go 参考实现；所有错误 `fmt.Errorf("...: %w", err)` 包装）
- [ ] **Step 5**: `go build ./... && go test ./...` 全绿
- [ ] **Step 6**: `git commit -m "refactor(repository): 引入数据访问层接口与 GORM 实现"`

### Task 3: auth 域服务化（AuthService + PasswordHasher）

**Files:**
- Create: `internal/service/auth/password.go`（`PasswordHasher` 接口 + `MD5PasswordHasher` 实现，逻辑取自原 `model.MD5Hash`）
- Create: `internal/service/auth/auth_service.go` + `auth_service_test.go`
- Modify: `internal/handler/auth/handler.go`（瘦身为 bind → svc → render；保留原 HTTP 状态码 400/401/409/500 行为）
- Modify: `internal/router/router.go`（auth handler 改为注入 AuthService，不再 `d.AdminH.DB()`——本任务先改为从 Deps 取 `AuthSvc`，Deps 增加字段，main.go 装配处同步修改）

**接口（Produces，后续任务引用）：**

```go
package auth

type PasswordHasher interface {
	Hash(plain string) string
	Verify(plain, hashed string) bool
}

// LoginResult / CurrentUserVO 定义在 internal/model/user_dto.go
type AuthService interface {
	Login(ctx context.Context, username, password string) (*model.LoginResult, error)
	Register(ctx context.Context, username, password string) (*model.LoginResult, error)
	CurrentUser(ctx context.Context, userID string) (*model.CurrentUserVO, error)
}
```

错误码契约（对照现有 handler）：账号不存在/密码错误 → `errs.NotLogin`；账号已存在 → `errs.Business`；注册入库失败 → `errs.WrapServer`。用户名/密码长度校验（`len>=2 / >=4`）留在 handler（参数校验职责），业务规则校验在 service 重复兜底。JWT 签发在 service 内完成（依赖 `pkg/jwt`）。

**Steps:**
- [ ] **Step 1**: 手写 `mockUserRepo`（实现 UserRepository），写 auth_service_test.go：登录成功/账号不存在/密码错误/注册冲突 4 用例；确认失败
- [ ] **Step 2**: 实现 password.go + auth_service.go，测试转绿
- [ ] **Step 3**: 瘦身 auth handler + router/Deps/main 装配点同步；`go build ./... && go test ./...`
- [ ] **Step 4**: `git commit -m "refactor(auth): 认证业务下沉 service 层并接口化"`

### Task 4: admin 域服务层（11 个 service 接口 + 实现）

**Files:**
- Create: `internal/service/admin/services.go`（Services 聚合结构，见上文）
- Create: `internal/service/admin/{dashboard,knowledge,document,chunk,ingestion_task,intent,mapping,user,trace,audit,sample_question}_service.go`
- Create: `internal/service/admin/knowledge_service_test.go`、`user_service_test.go`（手写 mock repo，表格驱动）
- Modify: `internal/model/` 新增各域 `*_dto.go`：把现有 handler 文件里的 req/VO 结构体与 DO→VO 转换函数原样迁入（导出命名：`KnowledgeBaseVO`、`KnowledgeBaseCreateReq`、`KnowledgeBaseDOToVO` 等；JSON tag 逐字不变）

**行为规范**：每个 service 方法的业务逻辑 = 现有对应 handler 私有方法去掉 HTTP 部分。特殊点：
- DocumentService.Upload：保留「查 KB → 存文件 → Create doc → Create task → 触发 ingestionEngine」流程；`io.Copy` 的 error 必须处理（现在被忽略）；Create doc 与 Create task 的两次写入错误都必须处理并回滚已写文件（os.Remove）。ingestion 引擎触发通过 `Ingestor interface { Enqueue(taskID string) }` 之类的现有启动方式抽象（先读 document.go 确认现有触发方式再定义）。
- DocumentService.Delete：级联软删 chunk 的 error 必须处理（现在被忽略）。
- IntentService/MappingService：保留缓存清除回调（现有 `CacheClearer` 接口挪到 service 层依赖）。
- AuditService.Write：补上 `WithContext`（现在缺失）。
- DashboardService：四个方法分别组合 DashboardRepository 的聚合结果，输出结构逐字段对照现有响应。
- TraceService：`getTraceDetailReal` 里第二个查询忽略 error 的问题修复（返回包装错误）。
- SampleQuestionService.ListPublic：修复忽略 error。

**Steps:**
- [ ] **Step 1**: 迁移 DTO/VO 到 model（原 handler 文件暂时保留别名引用以免大爆炸：`type knowledgeBaseVO = model.KnowledgeBaseVO`，Task 5 删除）
- [ ] **Step 2**: 写 knowledge_service_test.go（List 透传 total / Create 时 vector 失败短路 / Delete 时 vector nil 不 panic）与 user_service_test.go（创建重名冲突 / 改密码）；确认失败
- [ ] **Step 3**: 实现全部 service；`go build ./... && go test ./...`
- [ ] **Step 4**: `git commit -m "refactor(service): 管理后台业务下沉 service 层并接口化"`

### Task 5: admin handler 瘦身 + httpx

**Files:**
- Create: `internal/handler/httpx/httpx.go`（上文全文）+ `httpx_test.go`
- Modify: `internal/handler/admin/` 全部 12 个文件：删除 db 字段/DB()/全部 Set 链式 setter，Handler 只持有 `admin.Services`；每个方法改为 bind → svc → render；删除 Task 4 留下的类型别名与迁走的转换函数
- Modify: `internal/handler/admin/{intent_tree_test,query_term_mapping_test}.go`（纯函数测试跟随函数迁移调整 import 到 model 包）
- Modify: `cmd/server/main.go`（admin.NewHandler 改为传 Services，装配 service；此时 main 变胖没关系，Task 8 收敛）

**Steps:**
- [ ] **Step 1**: 写 httpx_test.go（PageFromQuery 默认值/越界、Error 渲染 AppError 的 code 透传——用 gin test context）；确认失败 → 实现 httpx → 转绿
- [ ] **Step 2**: 逐文件瘦身 admin handler（顺序：knowledge_base → document → chunk → ingestion_task → user_mgmt → intent_tree → query_term_mapping → sample_question → trace → audit_log → dashboard → admin.go 收尾）
- [ ] **Step 3**: `go build ./... && go test ./...` 全绿；grep 验证 `internal/handler/` 下无 `gorm` import
- [ ] **Step 4**: `git commit -m "refactor(handler): admin handler 瘦身，仅保留 HTTP 职责"`

### Task 6: chat + session 域重构

> ⚠️ **接口缺口（Task 2 审查确认）**：现有 session/memory 代码的会话查询带 `user_id` 归属条件（session_service.go 的 Rename/Delete/ListMessages/updateVote、memory_service.go 的 loadHistory/Count），而 Conversation/Message/Summary repository 首版接口未表达。本任务必须为涉及的方法增加 user 归属参数（如 `UpdateFieldsForUser(ctx, id, userID, updates)`、`SoftDeleteForUser`、`ListByConversationForUser`、`UpdateVoteForUser`），签名以现有代码的 WHERE 条件为准逐一对照，同步补 mysql 实现；**严禁丢掉任何 user_id 过滤条件（越权风险）**。

**Files:**
- Create: `internal/service/rag/trace_recorder.go`（`TraceRecorder` 接口：`StartRun/FinishRun/CancelByTaskID`，实现依赖 `repository.TraceRepository`；状态值用 `model.TraceStatus*` 常量）
- Create: `internal/handler/session/handler.go`（从 `internal/service/rag/session_service.go` 拆出 HTTP 部分）
- Modify: `internal/service/rag/session_service.go`（只留业务，依赖 Conversation/Message repository；`SessionService` 接口化）
- Modify: `internal/handler/chat/handler.go`（去掉 db 字段，注入 TraceRecorder；`"RUNNING"/"CANCELLED"` 等裸字符串换常量）
- Modify: `internal/router/router.go`（session 路由挂新 handler；不再 `rag.NewSessionHandler(d.AdminH.DB(), ...)`）

**Steps:**
- [ ] **Step 1**: 先读 chat/handler.go 与 session_service.go 全文，列出全部 DB 触点与响应结构
- [ ] **Step 2**: 写 trace_recorder 的单测（mock TraceRepository：StartRun 写入 RUNNING、CancelByTaskID 更新字段）；实现转绿
- [ ] **Step 3**: 拆分 session handler/service；原 `session_test.go` 的纯函数测试跟随迁移
- [ ] **Step 4**: `go build ./... && go test ./...`；grep 验证 chat/session 无 gorm import
- [ ] **Step 5**: `git commit -m "refactor(chat): 会话与追踪数据访问下沉 repository"`

### Task 7: rag/ingestion 服务内 DB 访问改走 repository

> ⚠️ **接口缺口（Task 2 审查确认）**：memory/summary 的「最近 N 条 user 角色消息」窗口（summary_service.go 的 `role='user' ORDER BY id DESC LIMIT keepTurns`）无法用现有 `ListRecent` 表达，本任务需增加 `ListRecentByRole(ctx, convID, userID, role string, limit int)` 之类的方法（签名对照现有 SQL），同步补 mysql 实现；user_id 条件同 Task 6 的警告，一个都不能丢。

**Files:**
- Modify: `internal/service/rag/memory_service.go`（db 字段 → Conversation/Message/Summary 三个 repository 接口；`role = 'user'` 换 `model.MsgRoleUser`）
- Modify: `internal/service/rag/intent_loader.go`（→ IntentNodeRepository.ListActive）
- Modify: `internal/service/rag/term_mapping.go`（→ TermMappingRepository.ListEnabled；saveCache 的 Marshal 错误记日志）
- Modify: `internal/service/rag/metadata_enrich.go`（→ Chunk/Document repository 的 FindByIDs）
- Modify: `internal/service/ingestion/engine.go`（→ IngestionTask/Document/KnowledgeBase repository；补齐缺失的 WithContext——repo 天然带；`config.Get()` 全局读取改为构造函数注入 DataDir）
- Modify: `internal/service/ingestion/indexer.go`（→ Chunk/IngestionTask/Document repository）
- Modify: `cmd/server/main.go` 装配点同步

**Steps:**
- [ ] **Step 1**: 逐文件替换（每个文件先读全文，repo 接口若缺方法先补接口+实现+该方法的 sqlite 测试）
- [ ] **Step 2**: `go build ./... && go test ./...`；grep 验证 `internal/service/` 下除 `repository/mysql` 外无 gorm import（注：`internal/service` 全域）
- [ ] **Step 3**: `git commit -m "refactor(rag,ingestion): 服务层数据访问全面下沉 repository"`

### Task 8: bootstrap 装配 + router 去 DB 化 + main 瘦身

**Files:**
- Create: `internal/bootstrap/bootstrap.go`（`App` 结构 + `New(cfg) (*App, error)`：初始化 logger/db/redis/snowflake → `mysql.New(db)` → services → handlers → `Run()` 启动 gin + 优雅关闭）
- Create: `internal/bootstrap/probe.go`（原 cmd/server/init.go 的 InitDB/InitRedis/InitMilvus/InitEmbedding/InitLLM 迁入，改名 `probeDB` 等）
- Modify: `cmd/server/main.go`（≤60 行：loadDotEnv → config.Load → bootstrap.New → app.Run）
- Delete: `cmd/server/init.go`、`cmd/server/health.go`（与 internal/router/health.go 重复；SettingsHandler 迁到 internal/router/health.go 或独立 settings handler）
- Modify: `internal/config/config.go`（删除 SetDBGorm/SetRedisClient/GetDB/GetRedis 四个服务定位器函数及 globalDB/globalRedis 变量；先 grep 确认无残余调用方）
- Modify: `internal/router/router.go`（Deps 只含 handler 与中间件；模块化拆分 registerAuth/registerChat/registerSession/registerAdmin/registerHealth 私有函数）
- Modify: `cmd/server/migrate.go`（driver 与主程序统一为 MySQL；`sqlDB, _ :=` 修复；main 中接上 `--migrate` 分支或删除该文件——读文件后择一，倾向接上）
- Fix: `main.go` 的 `db, _ := initDB(cfg)` —— error 必须记 zap.Warn 并继续（保持现有"无 DB 可启动"的降级行为，但不再静默）

**Steps:**
- [ ] **Step 1**: 建 bootstrap，main 内容整体迁移（装配代码从 main 移到 bootstrap.New）
- [ ] **Step 2**: router 模块化 + Deps 精简；删除重复文件
- [ ] **Step 3**: `go build ./... && go vet ./... && go test ./...` 全绿；`golangci-lint run ./...`（若本机可用）
- [ ] **Step 4**: `git commit -m "refactor(bootstrap): 依赖装配收敛，main 瘦身，路由去 DB 化"`

### Task 9: 收尾——文档 + 全量验证

**Files:**
- Modify: `docs/architecture.md`（更新目录树与分层说明）
- Create: `docs/superpowers/plans/2026-07-17-enterprise-layering-refactor.md` 已存在（本文件）；另 Create `docs/refactor-report.md`：重构前问题总结（对照审计清单）+ 重构后优势说明（交付给用户的报告素材）
- 全局 grep 复查：`gorm` import 仅存在于 `internal/repository/mysql/`、`internal/bootstrap/`、`cmd/server/`（migrate）；`internal/handler` 无业务逻辑残留；无 `magic` 状态裸字符串（对照 consts.go）

**Steps:**
- [ ] **Step 1**: 全局验证命令逐一执行并记录输出：`go build ./...`、`go vet ./...`、`go test ./...`、`grep -rn "gorm" internal/handler internal/service --include="*.go" | grep -v repository`（应为空）
- [ ] **Step 2**: 写 refactor-report.md
- [ ] **Step 3**: `git commit -m "docs: 更新架构文档与重构报告"`

---

## 验证方案（整体）

1. **编译/静态**：`go build ./... && go vet ./...`（+ golangci-lint run，若可用）
2. **单测**：`go test ./...` —— 新增：model/page、repo(sqlite)×2、auth service、admin service×2、httpx、trace_recorder；存量纯函数测试全部保活
3. **分层断言（grep 审计）**：
   - `internal/handler/**` 无 `gorm.io` import
   - `internal/service/**` 无 `gorm.io`、无 `gin-gonic` import
   - `internal/model/**` 只 import 标准库
   - `pkg/**` 无 `goRAGENT/internal` import
4. **API 兼容抽查**：对 knowledge-base 列表、trace 列表（current/size 风格）、login 三个端点，比对重构前后响应 JSON 结构（可用 handler 层 gin 测试或对照代码逐字段核对）
