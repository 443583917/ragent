# goRAGENT 开发规范与代码生成规则

> 版本: v1.0.0 | 最后更新: 2026-07-16

---

## 一、代码风格

### 强制工具链

```makefile
# Makefile 中的 lint 目标
lint:
	golangci-lint run ./...
	go vet ./...
```

`.golangci.yml`：

```yaml
linters:
  enable:
    - errcheck      # 必须检查 error 返回值
    - gosimple      # 简化代码建议
    - govet         # Go 官方静态分析
    - ineffassign   # 无效赋值检测
    - staticcheck   # 静态分析
    - unused        # 未使用变量/函数
    - gofmt         # 格式检查
    - goimports     # import 排序

linters-settings:
  gofmt:
    simplify: true
  errcheck:
    check-blank: true   # 禁止 _ 忽略 error
```

### 命名规范

| 范围 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写、单数、简短 | `intent` 非 `intents` |
| 导出类型 | CamelCase | `SearchChannel`, `NodeScore` |
| 非导出类型 | camelCase | `intentTree`, `searchContext` |
| 接口 | 单方法接口用 `-er` 后缀 | `Classifier`, `Embedder` |
| 常量 | CamelCase (非全大写) | `IntentMinScore` 非 `INTENT_MIN_SCORE` |
| 文件名 | snake_case | `intent_classifier.go` |
| 测试文件 | `*_test.go` | `classifier_test.go` |

### 包导入分组

```go
import (
    // 1. 标准库
    "context"
    "fmt"
    "time"

    // 2. 第三方
    "github.com/gin-gonic/gin"
    "github.com/redis/go-redis/v9"
    "trpc.group/trpc-go/trpc-agent-go/graph"

    // 3. 本项目
    "goRAGENT/internal/framework/sse"
    "goRAGENT/internal/rag/intent"
)
```

---

## 二、项目结构规范

### 新增模块规则

```
internal/<领域>/
├── <领域>.go          # 公开接口定义（interface + 公开函数）
├── <领域>_impl.go     # 接口实现（不导出）
├── config.go          # 配置 struct（需要外部读取时公开字段）
├── model.go           # 数据模型 struct（DO/DTO）
└── *_test.go          # 单元测试（同包）
```

### 禁止事项

- ❌ `utils/` 包 — 按功能归属，不要建万能工具包
- ❌ `common/` 包 — 同上
- ❌ 包名和目录名不一致
- ❌ 循环依赖（`go mod tidy` + `go build` 会报，但也禁止设计层面产生）

---

## 三、错误处理规范

### 标准模式

```go
// ✅ 逐层返回 + 上下文包装
func (s *Service) DoSomething(ctx context.Context, id string) (*Result, error) {
    data, err := s.repo.Find(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("DoSomething: find id=%s: %w", id, err)
    }
    return process(data), nil
}

// ❌ 不要吞错误
func Bad(x string) {
    result, _ := DoSomething(x)  // 禁止 _ 忽略 error
}

// ❌ 不要只返回 error 字符串
func Bad2() error {
    return errors.New("失败了")  // 没有上下文信息
}
```

### 自定义错误类型

```go
// pkg/errs/errs.go（统一分级错误类型）
type AppError struct {
    Code    string // 错误码 "A000001"
    Message string // 用户可读的描述
    Cause   error  // 原始 error（不暴露给前端）
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Cause }

// 常用构造
func NewBusinessError(msg string) *AppError {
    return &AppError{Code: "B000001", Message: msg}
}
```

---

## 四、并发规范

### goroutine 管理

```go
// ✅ 使用 errgroup 管理并发 + 错误聚合
g, ctx := errgroup.WithContext(ctx)
for _, item := range items {
    item := item
    g.Go(func() error {
        return process(ctx, item)
    })
}
if err := g.Wait(); err != nil {
    // 处理
}

// ✅ goroutine 内必须 recover
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error("goroutine panic", zap.Any("recover", r))
        }
    }()
    doWork()
}()

// ❌ 不控制 goroutine 数量
for _, item := range hugeList {
    go process(item)  // 如果 hugeList 有 10 万条，瞬间爆
}

// ✅ 用 semaphore 或 worker pool 限制并发
sem := make(chan struct{}, 10)
for _, item := range hugeList {
    sem <- struct{}{}
    go func(item Item) {
        defer func() { <-sem }()
        process(item)
    }(item)
}
```

### context 传递

```go
// ✅ context 作为第一个参数
func Search(ctx context.Context, query string) ([]Result, error)

// ✅ 跨 goroutine 传递 context
go func(ctx context.Context) {
    select {
    case <-ctx.Done():
        return
    case result := <-ch:
        process(result)
    }
}(ctx)

// ❌ 不要把 context 存到 struct 里
type BadService struct {
    ctx context.Context  // 禁止
}
```

---

## 五、配置管理规范

### 配置定义

```go
// 使用结构体 tag 映射 yaml 字段
type SearchConfig struct {
    DefaultTopK int `mapstructure:"default_top_k" yaml:"default_top_k"`
    Channels    struct {
        VectorGlobal VectorGlobalConfig `mapstructure:"vector_global" yaml:"vector_global"`
    } `mapstructure:"channels" yaml:"channels"`
}
```

### 配置注入

```go
// ✅ 在 main.go 中统一加载，注入到各组件构造函数
func main() {
    var cfg Config
    viper.Unmarshal(&cfg)

    llmService := llm.NewService(cfg.AI)
    engine := retrieve.NewEngine(cfg.Search)

    server := NewServer(cfg, llmService, engine)
    server.Run()
}

// ❌ 不要在业务代码里直接读配置
func Bad() {
    apiKey := viper.GetString("ai.providers.bailian.api_key")  // 禁止
}
```

---

## 六、测试规范

### 文件组织

```
internal/rag/intent/
├── classifier.go
├── classifier_test.go      # 单元测试，和源文件同目录
└── testdata/               # 测试数据
    └── intent_tree.json

tests/
├── e2e/
│   └── chat_test.go        # 端到端测试
└── bench/
    └── chat_bench_test.go  # 压测
```

### 测试函数命名

```go
// 格式: Test_{类型}_{方法}_{场景}
func TestClassifier_ClassifyTargets_MultipleIntents(t *testing.T) {}
func TestClassifier_ClassifyTargets_EmptyQuestion(t *testing.T) {}
func TestResolver_FilterScores_BelowThreshold(t *testing.T) {}

// 表格驱动测试（Go 惯用法）
func TestDedup_Process(t *testing.T) {
    tests := []struct {
        name     string
        input    []RetrievedChunk
        expected int  // 去重后数量
    }{
        {"all unique", threeUniqueChunks(), 3},
        {"all duplicates", threeSameChunks(), 1},
        {"mixed", mixedChunks(), 2},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Dedup{}.Process(tt.input)
            assert.Equal(t, tt.expected, len(result))
        })
    }
}
```

### Mock 策略

```go
// ✅ 用接口 + 手写 mock（不引入 mock 框架）
type Classifier interface {
    ClassifyTargets(ctx context.Context, question string) ([]NodeScore, error)
}

type mockClassifier struct {
    scores []NodeScore
    err    error
}

func (m *mockClassifier) ClassifyTargets(ctx context.Context, q string) ([]NodeScore, error) {
    return m.scores, m.err
}
```

---

## 七、代码生成规则

### 7.1 GORM gen（数据库 Model 生成）

```bash
# 安装 gen 工具
go install gorm.io/gen/tools/gentool@latest

# 生成 Model
gentool -dsn "postgres://user:pass@localhost:5432/ragent?sslmode=disable" \
  -tables "t_conversation,t_conversation_message,t_conversation_summary,t_intent_node,t_knowledge_base,..." \
  -outPath "./internal/repo/gen" \
  -modelPkgName "model"
```

生成的代码放在 `internal/repo/gen/`，**不手动修改**。业务代码通过 GORM 的 `db.Where().Find()` 查询，不直接依赖 gen 生成的方法。

### 7.2 go:embed（Prompt 模板嵌入）

```go
// internal/rag/prompt/loader.go
package prompt

import "embed"

//go:embed prompts/*
var templateFS embed.FS

func Load(name string) (string, error) {
    data, err := templateFS.ReadFile("prompts/" + name)
    return string(data), err
}
```

**规则**：模板文件更新后不需要重新生成代码，`go build` 会自动重新嵌入。

### 7.3 stringer（枚举类型 String() 自动生成）

```bash
go install golang.org/x/tools/cmd/stringer@latest
```

```go
// internal/rag/intent/node.go
//go:generate stringer -type=IntentKind -linecomment
type IntentKind int

const (
    KindKB     IntentKind = iota // KB
    KindMCP                      // MCP
    KindSystem                   // SYSTEM
)
```

运行 `go generate ./...` 自动生成 `intentkind_string.go`。

### 7.4 Wire（依赖注入生成 — 可选）

如果依赖太多不适合手动注入，引入 Google Wire：

```go
// cmd/server/wire.go
//go:build wireinject
// +build wireinject

func InitializeServer(cfg *Config) (*Server, error) {
    wire.Build(
        infra.NewLLMService,
        infra.NewEmbeddingService,
        rag.NewRetrievalEngine,
        rag.NewGraphAgent,
        admin.NewHandler,
        NewServer,
    )
    return nil, nil
}
```

```bash
wire ./cmd/server/
# 生成 wire_gen.go
```

规则：**只改 `wire.go`，然后运行 `wire` 生成，不手动改 `wire_gen.go`。**

---

## 八、Git 提交规范

```
<type>(<scope>): <简短描述>

类型:
  feat     新功能
  fix      Bug 修复
  refactor 重构（不改功能）
  perf     性能优化
  test     测试
  docs     文档
  chore    构建/工具

范围:
  pipeline, intent, retrieve, memory, admin, infra, mcp, ...

示例:
  feat(retrieve): 实现 IntentDirectedSearchChannel 向量检索
  fix(pipeline): 修复空意图时 GraphAgent 条件边路由错误
  refactor(memory): 简化历史加载并发模型
  test(intent): 补充意图分类边界用例
```

---

## 九、依赖版本管理

```bash
# 锁定所有依赖版本
go mod tidy
go mod verify

# 升级单个依赖
go get trpc.group/trpc-go/trpc-agent-go@v1.10.0

# 查看可用更新
go list -m -u all
```

- tRPC-Agent-Go 锁定 `v1.10.0`，不追 latest
- 其他依赖在 Phase 5 集中评估升级
- `go.sum` 必须提交

---

## 十、禁止清单

| ❌ 禁止 | ✅ 替代方案 |
|--------|-----------|
| `panic()` 用于正常错误处理 | `return error` |
| `_` 忽略 error | 显式处理或 `log.Warn` |
| 全局变量保存状态 | 结构体字段 + 构造函数注入 |
| `time.Sleep` 用于等待 | `time.Ticker` / `context.WithTimeout` / channel |
| 裸 goroutine 无退出控制 | ctx.Done() + select |
| 在循环里拼接字符串 | `strings.Builder` |
| JSON map[string]any 强转 | 定义 struct 反序列化 |
| init() 做重操作 | main() 或 lazy init |
