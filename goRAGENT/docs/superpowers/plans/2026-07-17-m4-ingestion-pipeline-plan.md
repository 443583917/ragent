# M4 文档入库 Pipeline 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实装知识库/文档/Chunk 管理 CRUD + 文档入库 Pipeline（MinerU 解析→混合切分→嵌入→Milvus 索引）+ 检索元数据富化后处理器。

**Architecture:** 新包 `internal/ingestion/` 实现 4 节点 Pipeline（Fetcher→Parser→Chunker→Indexer）通过 Engine 异步 goroutine 编排；`admin.Handler` 通过 Setter 注入 Engine 和 DataDir 后覆盖空壳路由；Milvus 补写操作；检索路径加 MetadataEnrichment 后处理器回表补文档元数据。

**Tech Stack:** Go + GORM + Gin + Milvus Go SDK v2 + BGE-M3 Embedding HTTP + MinerU v1 API

**Spec:** `docs/superpowers/specs/2026-07-17-m4-ingestion-pipeline-design.md`

## Global Constraints

- 所有实体 ID 使用 snowflake VARCHAR(32)（`snowflake.NextID()`），仅 IngestionTaskDO 用 BIGINT AUTO_INCREMENT
- handler 模式对齐 `intent_tree.go`：`h.db.WithContext(ctx)` + `response.Success/Failure` + `userctx.GetUserID`
- 软删除统一用 `deleted` 字段 = 1
- 分页请求使用 `page`/`pageSize` query params，默认 page=1 pageSize=20
- Collection 命名：`kb_{snowflake_id}`
- 文件存储目录：`data/files/{kb_id}/{doc_id}.{ext}` + `data/parsed/{doc_id}.md`
- 环境变量前缀：`MINERU_`、`INGESTION_`
- Chunk 大小 1024 char，重叠 50 char；嵌入 batch size 32
- 所有新代码文件放在 `goRAGENT/` 目录下

---

### Task 1: init.sql — 修改现有表 + 新增表

**Files:**
- Modify: `goRAGENT/docker/init.sql`

**Interfaces:**
- Produces: `t_knowledge_base` id 改为 VARCHAR(32)，`t_document` id 改为 VARCHAR(32)，新增 `t_chunk` 表，新增 `t_ingestion_task` 表

- [ ] **Step 1: 改 t_knowledge_base 和 t_document 的 id 列类型**

在 `docker/init.sql` 中修改两个已有表定义。找到 `t_knowledge_base`（第94行）将 `id BIGINT AUTO_INCREMENT PRIMARY KEY` 改为 `id VARCHAR(32) PRIMARY KEY`。找到 `t_document`（第106行）同样修改。

- [ ] **Step 2: 加 t_chunk 和 t_ingestion_task 表**

在 `docker/init.sql` 末尾（`t_rag_trace_node` 之后）追加：

```sql
CREATE TABLE IF NOT EXISTS t_chunk (
    id              VARCHAR(32)  PRIMARY KEY,
    doc_id          VARCHAR(32)  NOT NULL,
    kb_id           VARCHAR(32)  NOT NULL,
    chunk_index     INT          NOT NULL,
    text            MEDIUMTEXT   NOT NULL,
    char_count      INT          DEFAULT 0,
    token_count     INT          DEFAULT 0,
    embedding_status VARCHAR(16) DEFAULT 'PENDING',
    enabled         TINYINT      NOT NULL DEFAULT 1,
    deleted         TINYINT      NOT NULL DEFAULT 0,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_ingestion_task (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    kb_id           VARCHAR(32)  NOT NULL,
    doc_id          VARCHAR(32)  NOT NULL,
    status          VARCHAR(16)  DEFAULT 'PENDING',
    total_chunks    INT          DEFAULT 0,
    completed_chunks INT         DEFAULT 0,
    error_message   TEXT,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- [ ] **Step 3: 验证 SQL 语法**

```bash
cd goRAGENT && docker exec -i ragent-mysql mysql -uroot -p123456 ragent -e "SELECT 1"
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/docker/init.sql
git commit -m "feat(m4): add t_chunk + t_ingestion_task tables, change kb/doc id to VARCHAR"
```

---

### Task 2: Go DO 模型 — `internal/rag/knowledge.go`

**Files:**
- Create: `goRAGENT/internal/rag/knowledge.go`

**Interfaces:**
- Produces: `KnowledgeBaseDO`, `DocumentDO`, `ChunkDO`, `IngestionTaskDO` 四个 GORM 模型 + 各自的 `TableName()` 方法

- [ ] **Step 1: 写入 knowledge.go**

```go
package rag

import "time"

// KnowledgeBaseDO t_knowledge_base 知识库
type KnowledgeBaseDO struct {
	ID             string    `gorm:"column:id;primaryKey"`
	Name           string    `gorm:"column:name"`
	Description    string    `gorm:"column:description"`
	EmbeddingModel string    `gorm:"column:embedding_model"`
	CollectionName string    `gorm:"column:collection_name"`
	Dimension      int       `gorm:"column:dimension"`
	Deleted        int       `gorm:"column:deleted"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (KnowledgeBaseDO) TableName() string { return "t_knowledge_base" }

// DocumentDO t_document 文档
type DocumentDO struct {
	ID         string    `gorm:"column:id;primaryKey"`
	KbID       string    `gorm:"column:kb_id"`
	FileName   string    `gorm:"column:file_name"`
	FileType   string    `gorm:"column:file_type"`
	FileSize   int64     `gorm:"column:file_size"`
	Status     string    `gorm:"column:status"`
	ChunkCount int       `gorm:"column:chunk_count"`
	Deleted    int       `gorm:"column:deleted"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (DocumentDO) TableName() string { return "t_document" }

// ChunkDO t_chunk 文档块
type ChunkDO struct {
	ID              string    `gorm:"column:id;primaryKey"`
	DocID           string    `gorm:"column:doc_id"`
	KbID            string    `gorm:"column:kb_id"`
	ChunkIndex      int       `gorm:"column:chunk_index"`
	Text            string    `gorm:"column:text"`
	CharCount       int       `gorm:"column:char_count"`
	TokenCount      int       `gorm:"column:token_count"`
	EmbeddingStatus string    `gorm:"column:embedding_status"`
	Enabled         int       `gorm:"column:enabled"`
	Deleted         int       `gorm:"column:deleted"`
	CreateTime      time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime      time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (ChunkDO) TableName() string { return "t_chunk" }

// IngestionTaskDO t_ingestion_task 入库任务
type IngestionTaskDO struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement"`
	KbID            string    `gorm:"column:kb_id"`
	DocID           string    `gorm:"column:doc_id"`
	Status          string    `gorm:"column:status"`
	TotalChunks     int       `gorm:"column:total_chunks"`
	CompletedChunks int       `gorm:"column:completed_chunks"`
	ErrorMessage    string    `gorm:"column:error_message"`
	CreateTime      time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime      time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (IngestionTaskDO) TableName() string { return "t_ingestion_task" }

// 入库相关常量
const (
	DocStatusPending    = "PENDING"
	DocStatusProcessing = "PROCESSING"
	DocStatusDone       = "DONE"
	DocStatusFailed     = "FAILED"

	TaskStatusPending = "PENDING"
	TaskStatusRunning = "RUNNING"
	TaskStatusDone    = "DONE"
	TaskStatusFailed  = "FAILED"

	EmbedStatusPending = "PENDING"
	EmbedStatusDone    = "DONE"
	EmbedStatusFailed  = "FAILED"
)
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/knowledge.go
git commit -m "feat(m4): add KnowledgeBaseDO/DocumentDO/ChunkDO/IngestionTaskDO models"
```

---

### Task 3: Config 扩展 — 加 IngestionConfig + MineruConfig.DataDir

**Files:**
- Modify: `goRAGENT/internal/config/config.go`

**Interfaces:**
- Consumes: nothing new
- Produces: `MineruConfig.DataDir string`, `IngestionConfig` struct, `Config.Ingestion IngestionConfig`

- [ ] **Step 1: 修改 MineruConfig 和新增 IngestionConfig**

在 `config.go` 的 `MineruConfig` 结构体中加 `DataDir` 字段；新增 `IngestionConfig` 结构体并嵌入 `Config`。找到 `type MineruConfig struct`（约165行）：

```go
type MineruConfig struct {
	APIToken string
	DataDir  string // 文件管理根目录，默认 "data"
}

type IngestionConfig struct {
	ChunkSize      int // 分块大小（字符），默认 1024
	ChunkOverlap   int // 重叠大小（字符），默认 50
	EmbedBatchSize int // 嵌入批大小，默认 32
}
```

在 `Config` 结构体（约32行）新增字段：
```go
type Config struct {
	// ... 已有字段 ...
	Ingestion IngestionConfig
}
```

- [ ] **Step 2: 在 `Load()` 中填充默认值**

在 `Load()` 函数中（`global = cfg` 之前），加入：

```go
Mineru: MineruConfig{
	APIToken: envStr("MINERU_API_TOKEN", ""),
	DataDir:  envStr("MINERU_DATA_DIR", "data"),
},
Ingestion: IngestionConfig{
	ChunkSize:      envInt("INGESTION_CHUNK_SIZE", 1024),
	ChunkOverlap:   envInt("INGESTION_CHUNK_OVERLAP", 50),
	EmbedBatchSize: envInt("INGESTION_EMBED_BATCH_SIZE", 32),
},
```

注意 `Mineru` 已经在 `Load()` 中存在（约326行），只需要在已有结构体中加上 `DataDir` 字段，同时在 `Config` 初始化时加入 `Ingestion` 字段。

- [ ] **Step 3: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/internal/config/config.go
git commit -m "feat(m4): add IngestionConfig + MineruConfig.DataDir"
```

---

### Task 4: Milvus 写操作 — 补 `vectorstore/milvus.go`

**Files:**
- Modify: `goRAGENT/internal/rag/retrieve/vectorstore/milvus.go`

**Interfaces:**
- Consumes: `entity` package from milvus-sdk-go
- Produces: `ChunkVector` struct, `HasCollection(ctx, name) (bool, error)`, `CreateCollection(ctx, name, dim) error`, `Insert(ctx, collection, chunks []ChunkVector) error`, `DropCollection(ctx, name) error`

- [ ] **Step 1: 加 `ChunkVector` 结构体**

在 `milvus.go` 的 import 块之后、`MilvusStore` 定义之前插入：

```go
// ChunkVector 待入库的向量化文档块
type ChunkVector struct {
	ID       string
	Text     string
	Metadata string // JSON string
	Vector   []float32
}
```

- [ ] **Step 2: 加 `HasCollection` 方法**

在 `MilvusStore` 的 `ListCollections` 方法之后追加：

```go
// HasCollection 检查 Collection 是否存在
func (m *MilvusStore) HasCollection(ctx context.Context, name string) (bool, error) {
	return m.client.HasCollection(ctx, name)
}
```

- [ ] **Step 3: 加 `CreateCollection` 方法**

```go
// CreateCollection 创建向量集合（COSINE 度量 + IVF_FLAT 索引）
func (m *MilvusStore) CreateCollection(ctx context.Context, name string, dim int) error {
	has, err := m.client.HasCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("检查 collection 失败: %w", err)
	}
	if has {
		zap.L().Info("collection 已存在，跳过创建", zap.String("name", name))
		return nil
	}

	schema := entity.NewSchema().
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dim)))

	if err := m.client.CreateCollection(ctx, name, schema, 2); err != nil {
		return fmt.Errorf("创建 collection 失败: %w", err)
	}

	// 建 IVF_FLAT 索引
	idx, err := entity.NewIndexIvfFlat(entity.COSINE, 128)
	if err != nil {
		return fmt.Errorf("创建索引定义失败: %w", err)
	}
	if err := m.client.CreateIndex(ctx, name, "vector", idx, false); err != nil {
		return fmt.Errorf("创建索引失败: %w", err)
	}

	zap.L().Info("collection 创建完成", zap.String("name", name), zap.Int("dim", dim))
	return nil
}
```

- [ ] **Step 4: 加 `Insert` 方法**

```go
// Insert 批量插入向量数据
func (m *MilvusStore) Insert(ctx context.Context, collection string, chunks []ChunkVector) error {
	if len(chunks) == 0 {
		return nil
	}

	// 构建列数据
	idCol := make([]string, len(chunks))
	textCol := make([]string, len(chunks))
	metaCol := make([]string, len(chunks))
	vecCol := make([][]float32, len(chunks))

	for i, c := range chunks {
		idCol[i] = c.ID
		textCol[i] = c.Text
		metaCol[i] = c.Metadata
		vecCol[i] = c.Vector
	}

	columns := []entity.Column{
		entity.NewColumnVarChar("id", idCol),
		entity.NewColumnVarChar("text", textCol),
		entity.NewColumnVarChar("metadata", metaCol),
		entity.NewColumnFloatVector("vector", len(chunks), vecCol...),
	}

	if _, err := m.client.Insert(ctx, collection, "", columns...); err != nil {
		return fmt.Errorf("Milvus Insert 失败: %w", err)
	}

	// 插入后刷新索引
	if err := m.client.Flush(ctx, collection, false); err != nil {
		zap.L().Warn("Milvus Flush 失败", zap.Error(err))
	}

	zap.L().Info("Milvus Insert 完成", zap.String("collection", collection), zap.Int("count", len(chunks)))
	return nil
}
```

- [ ] **Step 5: 加 `DropCollection` 方法**

```go
// DropCollection 删除向量集合
func (m *MilvusStore) DropCollection(ctx context.Context, name string) error {
	has, err := m.client.HasCollection(ctx, name)
	if err != nil || !has {
		return err
	}
	if err := m.client.DropCollection(ctx, name); err != nil {
		return fmt.Errorf("删除 collection 失败: %w", err)
	}
	zap.L().Info("collection 已删除", zap.String("name", name))
	return nil
}
```

- [ ] **Step 6: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add goRAGENT/internal/rag/retrieve/vectorstore/milvus.go
git commit -m "feat(m4): add Milvus write ops (HasCollection/CreateCollection/Insert/DropCollection)"
```

---

### Task 5: Ingestion Pipeline — Node 接口 + PipelineContext

**Files:**
- Create: `goRAGENT/internal/ingestion/node.go`

**Interfaces:**
- Produces: `Node` interface (`Name() string`, `Execute(ctx, pc) error`), `PipelineContext` struct, `ChunkSegment` struct

- [ ] **Step 1: 写入 node.go**

```go
package ingestion

import (
	"context"

	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
)

// Node 入库流水线节点接口
type Node interface {
	Name() string
	Execute(ctx context.Context, pc *PipelineContext) error
}

// PipelineContext 在节点间流转的上下文
type PipelineContext struct {
	Task     *rag.IngestionTaskDO
	KB       *rag.KnowledgeBaseDO
	Doc      *rag.DocumentDO
	FilePath string          // Fetcher 设置的文件路径
	Markdown string          // Parser 产出
	Chunks   []ChunkSegment  // Chunker 产出
}

// ChunkSegment 切分后的文本段（未嵌入）
type ChunkSegment struct {
	Index     int
	Text      string
	CharCount int
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/node.go
git commit -m "feat(m4): add ingestion Node interface + PipelineContext"
```

---

### Task 6: Ingestion Pipeline — Fetcher 节点

**Files:**
- Create: `goRAGENT/internal/ingestion/fetcher.go`

**Interfaces:**
- Consumes: `Node`, `PipelineContext` (from Task 5), `rag.DocumentDO` (from Task 2)
- Produces: `Fetcher` struct implementing `Node`

- [ ] **Step 1: 写入 fetcher.go**

```go
package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Fetcher 文件读取节点 — 按 doc 记录找到本地文件
type Fetcher struct {
	DataDir string
}

func (f *Fetcher) Name() string { return "Fetcher" }

func (f *Fetcher) Execute(ctx context.Context, pc *PipelineContext) error {
	ext := filepath.Ext(pc.Doc.FileName)
	if ext == "" {
		ext = ".txt"
	}
	filePath := filepath.Join(f.DataDir, "files", pc.Doc.KbID, pc.Doc.ID+ext)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	pc.FilePath = filePath
	return nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/fetcher.go
git commit -m "feat(m4): add ingestion Fetcher node"
```

---

### Task 7: Ingestion Pipeline — Parser 节点

**Files:**
- Create: `goRAGENT/internal/ingestion/parser.go`

**Interfaces:**
- Consumes: `Node`, `PipelineContext` (Task 5), `mineru.Client`
- Produces: `Parser` struct implementing `Node`

- [ ] **Step 1: 写入 parser.go**

```go
package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nageoffer/ragent/goRAGENT/internal/infra/mineru"
	"go.uber.org/zap"
)

// Parser 文档解析节点 — 文本文件直读，其他格式走 MinerU
type Parser struct {
	mineru  *mineru.Client
	DataDir string
}

func NewParser(mineruClient *mineru.Client, dataDir string) *Parser {
	return &Parser{mineru: mineruClient, dataDir: dataDir}
}

func (p *Parser) Name() string { return "Parser" }

func (p *Parser) Execute(ctx context.Context, pc *PipelineContext) error {
	ext := strings.ToLower(filepath.Ext(pc.Doc.FileName))

	var markdown string
	var err error

	switch ext {
	case ".md", ".markdown", ".txt":
		data, readErr := os.ReadFile(pc.FilePath)
		if readErr != nil {
			return fmt.Errorf("读取文本文件失败: %w", readErr)
		}
		markdown = string(data)
	default:
		// PDF/DOCX/PPT/图片 → MinerU v1
		markdown, err = p.mineru.Parse(ctx, pc.FilePath)
		if err != nil {
			return fmt.Errorf("MinerU 解析失败: %w", err)
		}
	}

	pc.Markdown = markdown

	// 解析产物落盘到 data/parsed/
	parsedDir := filepath.Join(p.DataDir, "parsed")
	os.MkdirAll(parsedDir, 0755)
	parsedPath := filepath.Join(parsedDir, pc.Doc.ID+".md")
	if err := os.WriteFile(parsedPath, []byte(markdown), 0644); err != nil {
		zap.L().Warn("保存解析产物失败", zap.String("path", parsedPath), zap.Error(err))
	}

	zap.L().Info("文档解析完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("chars", len(markdown)),
	)
	return nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/parser.go
git commit -m "feat(m4): add ingestion Parser node (text direct, MinerU for PDF)"
```

---

### Task 8: Ingestion Pipeline — Chunker 节点

**Files:**
- Create: `goRAGENT/internal/ingestion/chunker.go`

**Interfaces:**
- Consumes: `Node`, `PipelineContext`, `ChunkSegment` (Task 5), `config.IngestionConfig` (Task 3)
- Produces: `Chunker` struct implementing `Node`

- [ ] **Step 1: 写入 chunker.go**

```go
package ingestion

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"go.uber.org/zap"
)

// Chunker Markdown 标题混合切分节点
type Chunker struct {
	ChunkSize    int // 分块字符数上限
	ChunkOverlap int // 块间重叠字符数
}

func NewChunker(cfg config.IngestionConfig) *Chunker {
	return &Chunker{ChunkSize: cfg.ChunkSize, ChunkOverlap: cfg.ChunkOverlap}
}

func (c *Chunker) Name() string { return "Chunker" }

func (c *Chunker) Execute(ctx context.Context, pc *PipelineContext) error {
	markdown := pc.Markdown

	// Step 1: 按 # 标题分割 section
	sections := splitByHeading(markdown)

	// Step 2: 超过 ChunkSize 的 section 递归按固定大小切
	var chunks []ChunkSegment
	for _, sec := range sections {
		secLen := utf8.RuneCountInString(sec)
		if secLen <= c.ChunkSize {
			if strings.TrimSpace(sec) != "" {
				chunks = append(chunks, ChunkSegment{Index: len(chunks), Text: sec, CharCount: secLen})
			}
		} else {
			subs := splitFixedSize(sec, c.ChunkSize, c.ChunkOverlap)
			for _, sub := range subs {
				subLen := utf8.RuneCountInString(sub)
				if strings.TrimSpace(sub) != "" {
					chunks = append(chunks, ChunkSegment{Index: len(chunks), Text: sub, CharCount: subLen})
				}
			}
		}
	}

	pc.Chunks = chunks
	zap.L().Info("文档切分完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("chunks", len(chunks)),
	)
	return nil
}

// splitByHeading 按 Markdown 标题 (# ## ### ...) 分割
func splitByHeading(text string) []string {
	var sections []string
	lines := strings.Split(text, "\n")
	var buf strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "#") && buf.Len() > 0 {
			sections = append(sections, buf.String())
			buf.Reset()
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if buf.Len() > 0 {
		sections = append(sections, buf.String())
	}
	if len(sections) == 0 {
		sections = []string{text}
	}
	return sections
}

// splitFixedSize 固定大小切分（带重叠）
func splitFixedSize(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 1024
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 10
	}

	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/chunker.go
git commit -m "feat(m4): add ingestion Chunker node (heading + fixed-size hybrid)"
```

---

### Task 9: Ingestion Pipeline — Indexer 节点

**Files:**
- Create: `goRAGENT/internal/ingestion/indexer.go`

**Interfaces:**
- Consumes: `Node`, `PipelineContext`, `ChunkSegment` (Task 5), `embedding.Service`, `vectorstore.MilvusStore`, `rag.ChunkDO` (Task 2), `snowflake.NextID`, `gorm.DB`
- Produces: `Indexer` struct implementing `Node`

- [ ] **Step 1: 写入 indexer.go**

```go
package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nageoffer/ragent/goRAGENT/internal/framework/snowflake"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/embedding"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve/vectorstore"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Indexer 嵌入+索引+落库节点
type Indexer struct {
	db       *gorm.DB
	embed    *embedding.Service
	milvus   *vectorstore.MilvusStore
	batchSize int
}

func NewIndexer(db *gorm.DB, embedSvc *embedding.Service, milvusStore *vectorstore.MilvusStore, batchSize int) *Indexer {
	return &Indexer{db: db, embed: embedSvc, milvus: milvusStore, batchSize: batchSize}
}

func (idx *Indexer) Name() string { return "Indexer" }

func (idx *Indexer) Execute(ctx context.Context, pc *PipelineContext) error {
	if len(pc.Chunks) == 0 {
		return fmt.Errorf("没有可入库的 chunk")
	}

	batchSize := idx.batchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	// 收集所有文本
	texts := make([]string, len(pc.Chunks))
	for i, c := range pc.Chunks {
		texts[i] = c.Text
	}

	// 批量嵌入 + 分批入 Milvus + 分批写 MySQL
	totalChunks := len(pc.Chunks)
	for start := 0; start < totalChunks; start += batchSize {
		end := start + batchSize
		if end > totalChunks {
			end = totalChunks
		}

		batchTexts := texts[start:end]
		batchChunks := pc.Chunks[start:end]

		// 1. 批量向量化
		vectors, err := idx.embed.EmbedBatch(ctx, batchTexts)
		if err != nil {
			return fmt.Errorf("embed batch 失败 (offset=%d): %w", start, err)
		}

		// 2. 构建 Milvus ChunkVector + MySQL ChunkDO
		milvusChunks := make([]vectorstore.ChunkVector, len(batchChunks))
		mysqlChunks := make([]rag.ChunkDO, len(batchChunks))

		for i, chunk := range batchChunks {
			chunkID := snowflake.NextID()
			meta, _ := json.Marshal(map[string]any{
				"doc_id":    pc.Doc.ID,
				"kb_id":     pc.Doc.KbID,
				"file_name": pc.Doc.FileName,
				"chunk_index": chunk.Index,
			})

			milvusChunks[i] = vectorstore.ChunkVector{
				ID:       chunkID,
				Text:     chunk.Text,
				Metadata: string(meta),
				Vector:   vectors[i],
			}

			mysqlChunks[i] = rag.ChunkDO{
				ID:              chunkID,
				DocID:           pc.Doc.ID,
				KbID:            pc.Doc.KbID,
				ChunkIndex:      chunk.Index,
				Text:            chunk.Text,
				CharCount:       chunk.CharCount,
				EmbeddingStatus: rag.EmbedStatusDone,
				Enabled:         1,
			}
		}

		// 3. Milvus Insert
		if err := idx.milvus.Insert(ctx, pc.KB.CollectionName, milvusChunks); err != nil {
			return fmt.Errorf("Milvus Insert 失败 (offset=%d): %w", start, err)
		}

		// 4. MySQL 批量写入
		if err := idx.db.WithContext(ctx).Create(&mysqlChunks).Error; err != nil {
			return fmt.Errorf("MySQL 写入 t_chunk 失败 (offset=%d): %w", start, err)
		}

		// 5. 更新 task 进度
		idx.db.WithContext(ctx).Model(pc.Task).
			Update("completed_chunks", end)
	}

	// 更新 t_document.chunk_count 和状态
	idx.db.WithContext(ctx).Model(pc.Doc).Updates(map[string]any{
		"chunk_count": totalChunks,
		"status":      rag.DocStatusDone,
	})

	zap.L().Info("Indexer 入库完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("total_chunks", totalChunks),
	)
	return nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/indexer.go
git commit -m "feat(m4): add ingestion Indexer node (embed + Milvus insert + MySQL write)"
```

---

### Task 10: Ingestion Pipeline — Engine（异步编排）

**Files:**
- Create: `goRAGENT/internal/ingestion/engine.go`

**Interfaces:**
- Consumes: `gorm.DB`, `mineru.Client`, `embedding.Service`, `vectorstore.MilvusStore`, all 4 nodes (Tasks 6-9), `rag.*DO` (Task 2), `config.IngestionConfig`
- Produces: `Engine.Run(taskID int64)` — 异步 goroutine 内执行节点链

- [ ] **Step 1: 写入 engine.go**

```go
package ingestion

import (
	"context"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/embedding"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/mineru"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve/vectorstore"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Engine 入库流水线引擎
type Engine struct {
	db     *gorm.DB
	nodes  []Node
}

// NewEngine 创建引擎（注入依赖 + 组装节点链）
func NewEngine(db *gorm.DB, mineruClient *mineru.Client, embedSvc *embedding.Service, milvusStore *vectorstore.MilvusStore, cfg config.IngestionConfig) *Engine {
	dataDir := config.Get().Mineru.DataDir
	if dataDir == "" {
		dataDir = "data"
	}

	return &Engine{
		db: db,
		nodes: []Node{
			&Fetcher{DataDir: dataDir},
			NewParser(mineruClient, dataDir),
			NewChunker(cfg),
			NewIndexer(db, embedSvc, milvusStore, cfg.EmbedBatchSize),
		},
	}
}

// Run 异步执行入库 Pipeline
func (e *Engine) Run(taskID int64) {
	go func() {
		ctx := context.Background()
		var task rag.IngestionTaskDO

		// 1. 加载 task + doc + kb
		if err := e.db.First(&task, taskID).Error; err != nil {
			zap.L().Error("查询入库任务失败", zap.Int64("task_id", taskID), zap.Error(err))
			return
		}

		var doc rag.DocumentDO
		if err := e.db.First(&doc, "id = ?", task.DocID).Error; err != nil {
			zap.L().Error("查询文档失败", zap.String("doc_id", task.DocID), zap.Error(err))
			return
		}

		var kb rag.KnowledgeBaseDO
		if err := e.db.First(&kb, "id = ?", task.KbID).Error; err != nil {
			zap.L().Error("查询知识库失败", zap.String("kb_id", task.KbID), zap.Error(err))
			return
		}

		// 2. 更新状态为 RUNNING
		e.db.Model(&task).Updates(map[string]any{"status": rag.TaskStatusRunning})
		e.db.Model(&doc).Updates(map[string]any{"status": rag.DocStatusProcessing})

		// 3. 构建上下文 + 顺序执行节点
		pc := &PipelineContext{Task: &task, KB: &kb, Doc: &doc}

		for _, node := range e.nodes {
			t0 := time.Now()
			zap.L().Info("入库节点开始", zap.String("node", node.Name()), zap.Int64("task_id", taskID))

			if err := node.Execute(ctx, pc); err != nil {
				zap.L().Error("入库节点失败",
					zap.String("node", node.Name()),
					zap.Int64("task_id", taskID),
					zap.Error(err),
				)
				e.db.Model(&task).Updates(map[string]any{
					"status":        rag.TaskStatusFailed,
					"error_message": err.Error(),
				})
				e.db.Model(&doc).Updates(map[string]any{"status": rag.DocStatusFailed})
				return
			}

			zap.L().Info("入库节点完成",
				zap.String("node", node.Name()),
				zap.Duration("latency", time.Since(t0)),
			)
		}

		// 4. 全部成功
		e.db.Model(&task).Updates(map[string]any{
			"status":       rag.TaskStatusDone,
			"total_chunks": len(pc.Chunks),
		})

		zap.L().Info("入库 Pipeline 完成",
			zap.Int64("task_id", taskID),
			zap.String("doc_id", doc.ID),
			zap.Int("chunks", len(pc.Chunks)),
		)
	}()
}
```

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/ingestion/engine.go
git commit -m "feat(m4): add ingestion Engine (async goroutine pipeline)"
```

---

### Task 11: Admin — 知识库 CRUD 实装

**Files:**
- Create: `goRAGENT/internal/admin/knowledge_base.go`

**Interfaces:**
- Consumes: `admin.Handler`, `rag.KnowledgeBaseDO` (Task 2), `vectorstore.MilvusStore`, `snowflake.NextID`
- Produces: 覆盖 admin.go 中的 5 个空壳方法

- [ ] **Step 1: 先给 `admin.Handler` 注入 Milvus 依赖**

在 `admin/admin.go` 的 `Handler` 结构体中加字段（用于 Task 11-15）：

```go
// 在 import 中加:
// "github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve/vectorstore"

type Handler struct {
	db              *gorm.DB
	intentCache     CacheClearer
	mappingCache    CacheClearer
	milvus          *vectorstore.MilvusStore  // 新增
	ingestionEngine *ingestion.Engine         // 新增（Task 12 需要）
	dataDir         string                    // 新增
}
```

加 Setter 方法：
```go
func (h *Handler) SetMilvusStore(m *vectorstore.MilvusStore) *Handler {
	h.milvus = m
	return h
}
func (h *Handler) SetIngestionEngine(e *ingestion.Engine) *Handler {
	h.ingestionEngine = e
	return h
}
func (h *Handler) SetDataDir(d string) *Handler {
	h.dataDir = d
	return h
}
```

- [ ] **Step 2: 写入 knowledge_base.go — 列表 + 创建 + 详情 + 更新 + 删除**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/snowflake"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"go.uber.org/zap"
)

// ========== VO ==========

type knowledgeBaseVO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	EmbeddingModel string `json:"embeddingModel,omitempty"`
	CollectionName string `json:"collectionName,omitempty"`
	Dimension      int    `json:"dimension"`
	CreateTime     string `json:"createTime"`
}

type knowledgeBaseCreateReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type knowledgeBaseUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func kbDOtoVO(d rag.KnowledgeBaseDO) knowledgeBaseVO {
	return knowledgeBaseVO{
		ID: d.ID, Name: d.Name, Description: d.Description,
		EmbeddingModel: d.EmbeddingModel, CollectionName: d.CollectionName,
		Dimension: d.Dimension, CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}

// ========== Handlers ==========

func (h *Handler) listKnowledgeBases(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	offset := (page - 1) * pageSize

	var dos []rag.KnowledgeBaseDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.KnowledgeBaseDO{}).Where("deleted = 0").Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("deleted = 0").Order("create_time DESC").Offset(offset).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询知识库列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]knowledgeBaseVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, kbDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) createKnowledgeBase(c *gin.Context) {
	var req knowledgeBaseCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "name 不能为空"))
		return
	}

	id := snowflake.NextID()
	collectionName := "kb_" + id

	// 建 Milvus Collection
	if h.milvus != nil {
		if err := h.milvus.CreateCollection(c.Request.Context(), collectionName, 1536); err != nil {
			zap.L().Error("创建 Milvus Collection 失败", zap.Error(err))
			c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建向量集合失败"))
			return
		}
	}

	do := rag.KnowledgeBaseDO{
		ID: id, Name: req.Name, Description: req.Description,
		CollectionName: collectionName, Dimension: 1536,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(kbDOtoVO(do)))
}

func (h *Handler) getKnowledgeBase(c *gin.Context) {
	var do rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(kbDOtoVO(do)))
}

func (h *Handler) updateKnowledgeBase(c *gin.Context) {
	var req knowledgeBaseUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}

	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.KnowledgeBaseDO{}).
		Where("id = ? AND deleted = 0", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteKnowledgeBase(c *gin.Context) {
	id := c.Param("id")

	var do rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", id).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}

	// 删 Milvus Collection
	if h.milvus != nil && do.CollectionName != "" {
		h.milvus.DropCollection(c.Request.Context(), do.CollectionName)
	}

	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.KnowledgeBaseDO{}).
		Where("id = ?", id).
		Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除知识库失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
```

- [ ] **Step 3: 更新 admin.go 路由指向真实方法**

在 `admin.go` 中修改 5 个空壳的 Jump 函数：

```go
func (h *Handler) ListKnowledgeBases(c *gin.Context)  { h.listKnowledgeBases(c) }
func (h *Handler) CreateKnowledgeBase(c *gin.Context) { h.createKnowledgeBase(c) }
func (h *Handler) GetKnowledgeBase(c *gin.Context)    { h.getKnowledgeBase(c) }
func (h *Handler) UpdateKnowledgeBase(c *gin.Context)  { h.updateKnowledgeBase(c) }
func (h *Handler) DeleteKnowledgeBase(c *gin.Context)  { h.deleteKnowledgeBase(c) }
```

- [ ] **Step 4: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add goRAGENT/internal/admin/admin.go goRAGENT/internal/admin/knowledge_base.go
git commit -m "feat(m4): implement knowledge base CRUD with Milvus collection lifecycle"
```

---

### Task 12: Admin — 文档 CRUD + 上传

**Files:**
- Create: `goRAGENT/internal/admin/document.go`

**Interfaces:**
- Consumes: `admin.Handler`, `rag.DocumentDO` (Task 2), `ingestion.Engine` (Task 10), `snowflake.NextID`
- Produces: 覆盖 admin.go 中 8 个文档相关空壳 + 删除端点

- [ ] **Step 1: 写入 document.go**

```go
package admin

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/snowflake"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"go.uber.org/zap"
)

// ========== VO ==========

type documentVO struct {
	ID         string `json:"id"`
	KbID       string `json:"kbId"`
	FileName   string `json:"fileName"`
	FileType   string `json:"fileType"`
	FileSize   int64  `json:"fileSize"`
	Status     string `json:"status"`
	ChunkCount int    `json:"chunkCount"`
	CreateTime string `json:"createTime"`
}

func docDOtoVO(d rag.DocumentDO) documentVO {
	return documentVO{
		ID: d.ID, KbID: d.KbID, FileName: d.FileName, FileType: d.FileType,
		FileSize: d.FileSize, Status: d.Status, ChunkCount: d.ChunkCount,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}

// ========== Handlers ==========

func (h *Handler) listDocuments(c *gin.Context) {
	kbID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.DocumentDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).
		Where("kb_id = ? AND deleted = 0", kbID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("kb_id = ? AND deleted = 0", kbID).
		Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询文档列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]documentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, docDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) uploadDocument(c *gin.Context) {
	kbID := c.Param("id")

	// 校验知识库存在
	var kb rag.KnowledgeBaseDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", kbID).First(&kb).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "知识库不存在"))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "请选择文件"))
		return
	}
	defer file.Close()

	// 生成 doc ID
	docID := snowflake.NextID()
	ext := filepath.Ext(header.Filename)

	// 创建文件存储目录
	fileDir := filepath.Join(h.dataDir, "files", kbID)
	os.MkdirAll(fileDir, 0755)

	// 保存文件
	destPath := filepath.Join(fileDir, docID+ext)
	dst, err := os.Create(destPath)
	if err != nil {
		zap.L().Error("创建文件失败", zap.String("path", destPath), zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "文件保存失败"))
		return
	}
	defer dst.Close()

	written, _ := io.Copy(dst, file)

	// 写 t_document
	doc := rag.DocumentDO{
		ID: docID, KbID: kbID, FileName: header.Filename,
		FileType: ext, FileSize: written, Status: rag.DocStatusPending,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&doc).Error; err != nil {
		zap.L().Error("创建文档记录失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建文档失败"))
		return
	}

	// 创建入库任务
	task := rag.IngestionTaskDO{
		KbID: kbID, DocID: docID, Status: rag.TaskStatusPending,
	}
	h.db.WithContext(c.Request.Context()).Create(&task)

	// 异步启动入库
	if h.ingestionEngine != nil {
		h.ingestionEngine.Run(task.ID)
	}

	c.JSON(http.StatusOK, response.Success(docDOtoVO(doc)))
}

func (h *Handler) searchDocuments(c *gin.Context) {
	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.DocumentDO
	var total int64
	query := h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("deleted = 0")
	if keyword != "" {
		query = query.Where("file_name LIKE ?", "%"+keyword+"%")
	}
	query.Count(&total)
	if err := query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&dos).Error; err != nil {
		zap.L().Error("搜索文档失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "搜索失败"))
		return
	}

	vos := make([]documentVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, docDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(docDOtoVO(do)))
}

func (h *Handler) previewDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	parsedPath := filepath.Join(h.dataDir, "parsed", do.ID+".md")
	data, err := os.ReadFile(parsedPath)
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "解析产物尚未生成"))
		return
	}

	content := string(data)
	// 截取前 5000 字符
	runes := []rune(content)
	if len(runes) > 5000 {
		content = string(runes[:5000]) + "\n\n... (内容过长，已截断)"
	}

	c.JSON(http.StatusOK, response.Success(gin.H{"content": content, "docName": do.FileName}))
}

func (h *Handler) downloadDocument(c *gin.Context) {
	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	filePath := filepath.Join(h.dataDir, "files", do.KbID, do.ID+ext)

	c.Header("Content-Disposition", "attachment; filename=\""+do.FileName+"\"")
	c.File(filePath)
}

func (h *Handler) deleteDocument(c *gin.Context) {
	id := c.Param("id")

	var do rag.DocumentDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", id).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "文档不存在"))
		return
	}

	// 软删文档 + 关联 chunk
	h.db.WithContext(c.Request.Context()).Model(&rag.DocumentDO{}).Where("id = ?", id).Update("deleted", 1)
	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).Where("doc_id = ?", id).Update("deleted", 1)

	// 清理文件
	ext := filepath.Ext(do.FileName)
	if ext == "" {
		ext = ".txt"
	}
	os.Remove(filepath.Join(h.dataDir, "files", do.KbID, id+ext))
	os.Remove(filepath.Join(h.dataDir, "parsed", id+".md"))

	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) toggleDocument(c *gin.Context) {
	id := c.Param("docId")

	// 查任意一个 chunk 的当前 enabled 状态，取反后批量切换
	var sample rag.ChunkDO
	currentEnabled := 0
	if err := h.db.WithContext(c.Request.Context()).
		Where("doc_id = ? AND deleted = 0", id).First(&sample).Error; err == nil {
		currentEnabled = sample.Enabled
	}
	newEnabled := 0
	if currentEnabled == 0 {
		newEnabled = 1
	}

	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).
		Where("doc_id = ? AND deleted = 0", id).
		Update("enabled", newEnabled)
	c.JSON(http.StatusOK, response.Success(gin.H{"enabled": newEnabled}))
}
```

- [ ] **Step 2: 更新 admin.go 路由指向真实方法**

在 `admin.go` 中更新：
```go
func (h *Handler) ListDocuments(c *gin.Context)      { h.listDocuments(c) }
func (h *Handler) UploadDocument(c *gin.Context)      { h.uploadDocument(c) }
func (h *Handler) SearchDocuments(c *gin.Context)     { h.searchDocuments(c) }
func (h *Handler) GetDocument(c *gin.Context)         { h.getDocument(c) }
func (h *Handler) PreviewDocument(c *gin.Context)     { h.previewDocument(c) }
func (h *Handler) DownloadDocument(c *gin.Context)    { h.downloadDocument(c) }

// 在 admin.go 的 RegisterRoutes 中加删除路由和启停路由：
// kb.DELETE("/docs/:id", h.DeleteDocument)
// kb.PATCH("/docs/:docId/enable", h.ToggleDocument)
```

同时加公开方法：
```go
func (h *Handler) DeleteDocument(c *gin.Context) { h.deleteDocument(c) }
func (h *Handler) ToggleDocument(c *gin.Context) { h.toggleDocument(c) }
```

- [ ] **Step 3: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/internal/admin/admin.go goRAGENT/internal/admin/document.go
git commit -m "feat(m4): implement document CRUD + upload + preview + download"
```

---

### Task 13: Admin — Chunk CRUD

**Files:**
- Create: `goRAGENT/internal/admin/chunk.go`

**Interfaces:**
- Consumes: `rag.ChunkDO` (Task 2)
- Produces: 覆盖 admin.go 中 chunk 相关空壳

- [ ] **Step 1: 写入 chunk.go**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type chunkVO struct {
	ID              string `json:"id"`
	DocID           string `json:"docId"`
	KbID            string `json:"kbId"`
	ChunkIndex      int    `json:"chunkIndex"`
	Text            string `json:"text"`
	CharCount       int    `json:"charCount"`
	TokenCount      int    `json:"tokenCount"`
	EmbeddingStatus string `json:"embeddingStatus"`
	Enabled         int    `json:"enabled"`
}

func chunkDOtoVO(d rag.ChunkDO) chunkVO {
	return chunkVO{
		ID: d.ID, DocID: d.DocID, KbID: d.KbID, ChunkIndex: d.ChunkIndex,
		Text: d.Text, CharCount: d.CharCount, TokenCount: d.TokenCount,
		EmbeddingStatus: d.EmbeddingStatus, Enabled: d.Enabled,
	}
}

// listChunksByKB 按知识库分页
func (h *Handler) listChunksByKB(c *gin.Context) {
	kbID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.ChunkDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).
		Where("kb_id = ? AND deleted = 0", kbID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("kb_id = ? AND deleted = 0", kbID).
		Order("chunk_index ASC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]chunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, chunkDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

// listChunks 按 doc_id 分页（datasets 路径）
func (h *Handler) listChunks(c *gin.Context) {
	docID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.ChunkDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.ChunkDO{}).
		Where("doc_id = ? AND deleted = 0", docID).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Where("doc_id = ? AND deleted = 0", docID).
		Order("chunk_index ASC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询 Chunk 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]chunkVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, chunkDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getChunk(c *gin.Context) {
	var do rag.ChunkDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Chunk 不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(chunkDOtoVO(do)))
}

type chunkUpdateReq struct {
	Text *string `json:"text"`
}

func (h *Handler) updateChunk(c *gin.Context) {
	var req chunkUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := map[string]any{}
	if req.Text != nil {
		updates["text"] = *req.Text
		updates["char_count"] = len([]rune(*req.Text))
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}
	if err := h.db.WithContext(c.Request.Context()).
		Model(&rag.ChunkDO{}).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新 Chunk 失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) toggleChunk(c *gin.Context) {
	var do rag.ChunkDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("chunkId")).First(&do).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Chunk 不存在"))
		return
	}
	newEnabled := 0
	if do.Enabled == 0 {
		newEnabled = 1
	}
	h.db.WithContext(c.Request.Context()).Model(&do).Update("enabled", newEnabled)
	c.JSON(http.StatusOK, response.Success(gin.H{"enabled": newEnabled}))
}
```

- [ ] **Step 2: 更新 admin.go 跳转 + 路由**

```go
func (h *Handler) ListChunksByKB(c *gin.Context)  { h.listChunksByKB(c) }
func (h *Handler) ListChunks(c *gin.Context)      { h.listChunks(c) }
func (h *Handler) GetChunk(c *gin.Context)        { h.getChunk(c) }
func (h *Handler) UpdateChunk(c *gin.Context)     { h.updateChunk(c) }
func (h *Handler) ToggleChunk(c *gin.Context)     { h.toggleChunk(c) }
```

在 `RegisterRoutes` 中确保有：
```go
kb.GET("/docs/:docId/chunks/:chunkId", h.GetChunk)
kb.PUT("/docs/:docId/chunks/:chunkId", h.UpdateChunk)
kb.PATCH("/docs/:docId/chunks/:chunkId/enable", h.ToggleChunk)
```

- [ ] **Step 3: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/internal/admin/admin.go goRAGENT/internal/admin/chunk.go
git commit -m "feat(m4): implement chunk CRUD + enable/disable toggle"
```

---

### Task 14: Admin — 入库任务监控 API

**Files:**
- Create: `goRAGENT/internal/admin/ingestion_task.go`

**Interfaces:**
- Consumes: `rag.IngestionTaskDO` (Task 2)
- Produces: 覆盖 admin.go 中 4 个 ingestion 相关空壳

- [ ] **Step 1: 写入 ingestion_task.go**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type ingestionTaskVO struct {
	ID              int64  `json:"id"`
	KbID            string `json:"kbId"`
	DocID           string `json:"docId"`
	Status          string `json:"status"`
	TotalChunks     int    `json:"totalChunks"`
	CompletedChunks int    `json:"completedChunks"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	CreateTime      string `json:"createTime"`
}

type ingestionNodeVO struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // DONE / RUNNING / PENDING / FAILED
	Duration int64  `json:"duration"`
}

func taskDOtoVO(d rag.IngestionTaskDO) ingestionTaskVO {
	return ingestionTaskVO{
		ID: d.ID, KbID: d.KbID, DocID: d.DocID, Status: d.Status,
		TotalChunks: d.TotalChunks, CompletedChunks: d.CompletedChunks,
		ErrorMessage: d.ErrorMessage,
		CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
	}
}

func (h *Handler) listIngestionTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.IngestionTaskDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.IngestionTaskDO{}).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询入库任务列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]ingestionTaskVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, taskDOtoVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getIngestionTask(c *gin.Context) {
	var do rag.IngestionTaskDO
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.db.WithContext(c.Request.Context()).First(&do, id).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "任务不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(taskDOtoVO(do)))
}

func (h *Handler) getIngestionTaskNodes(c *gin.Context) {
	// M4 返回固定 4 节点状态（从 task 状态推导）
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var do rag.IngestionTaskDO
	if err := h.db.WithContext(c.Request.Context()).First(&do, id).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "任务不存在"))
		return
	}

	nodeNames := []string{"Fetcher", "Parser", "Chunker", "Indexer"}
	nodes := make([]ingestionNodeVO, 0, 4)
	for i, name := range nodeNames {
		status := "PENDING"
		switch do.Status {
		case rag.TaskStatusDone:
			status = "DONE"
		case rag.TaskStatusFailed:
			if do.CompletedChunks == 0 && i == 0 {
				status = "FAILED"
			} else if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else {
				status = "FAILED"
			}
		case rag.TaskStatusRunning:
			if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else if do.CompletedChunks > 0 {
				status = "RUNNING"
			} else if i == 0 {
				status = "RUNNING"
			}
		}
		nodes = append(nodes, ingestionNodeVO{Name: name, Status: status})
	}
	c.JSON(http.StatusOK, response.Success(nodes))
}
```

- [ ] **Step 2: 更新 admin.go 跳转**

```go
func (h *Handler) ListIngestionTasks(c *gin.Context)    { h.listIngestionTasks(c) }
func (h *Handler) GetIngestionTask(c *gin.Context)      { h.getIngestionTask(c) }
func (h *Handler) GetIngestionTaskNodes(c *gin.Context) { h.getIngestionTaskNodes(c) }
```

同时在 `RegisterRoutes` 中确保路由挂载这些方法。

- [ ] **Step 3: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add goRAGENT/internal/admin/admin.go goRAGENT/internal/admin/ingestion_task.go
git commit -m "feat(m4): implement ingestion task monitoring API"
```

---

### Task 15: 检索元数据富化后处理器

**Files:**
- Create: `goRAGENT/internal/rag/retrieve/postprocessor/metadata_enrich.go`

**Interfaces:**
- Consumes: `PostProcessor` interface (from `postprocessors.go`), `rag.ChunkDO` + `rag.DocumentDO` (Task 2), `gorm.DB`
- Produces: `MetadataEnrichmentPostProcessor` → `NewMetadataEnrichmentPostProcessor(db) *MetadataEnrichmentPostProcessor`

- [ ] **Step 1: 写入 metadata_enrich.go**

```go
package postprocessor

import (
	"context"

	"github.com/nageoffer/ragent/goRAGENT/internal/rag"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MetadataEnrichmentPostProcessor 元数据富化后处理器（最先执行）
// 回表 t_chunk + t_document 补 docId/docName/fileName 到 chunk Metadata 上
type MetadataEnrichmentPostProcessor struct {
	db *gorm.DB
}

func NewMetadataEnrichmentPostProcessor(db *gorm.DB) *MetadataEnrichmentPostProcessor {
	return &MetadataEnrichmentPostProcessor{db: db}
}

func (p *MetadataEnrichmentPostProcessor) Name() string { return "MetadataEnrichment" }
func (p *MetadataEnrichmentPostProcessor) Order() int   { return 0 }

func (p *MetadataEnrichmentPostProcessor) IsEnabled(ctx context.Context, sc *retrieve.SearchContext) bool {
	return true
}

func (p *MetadataEnrichmentPostProcessor) Process(ctx context.Context, chunks []retrieve.RetrievedChunk, sc *retrieve.SearchContext) ([]retrieve.RetrievedChunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}

	// 收集所有 chunk ID
	chunkIDs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.ID != "" {
			chunkIDs = append(chunkIDs, c.ID)
		}
	}
	if len(chunkIDs) == 0 {
		return chunks, nil
	}

	// 批量查 t_chunk 获取 doc_id
	var chunkDOs []rag.ChunkDO
	if err := p.db.WithContext(ctx).
		Select("id, doc_id").
		Where("id IN ? AND deleted = 0", chunkIDs).
		Find(&chunkDOs).Error; err != nil {
		zap.L().Warn("MetadataEnrichment 查 chunk 失败", zap.Error(err))
		return chunks, nil // 富化失败不影响检索主流程
	}

	chunkToDoc := make(map[string]string, len(chunkDOs))
	docIDSet := make(map[string]bool)
	for _, c := range chunkDOs {
		chunkToDoc[c.ID] = c.DocID
		docIDSet[c.DocID] = true
	}

	// 批量查 t_document 获取 file_name
	docIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		docIDs = append(docIDs, id)
	}

	var docDOs []rag.DocumentDO
	if len(docIDs) > 0 {
		p.db.WithContext(ctx).
			Select("id, file_name").
			Where("id IN ? AND deleted = 0", docIDs).
			Find(&docDOs)
	}

	docToName := make(map[string]string, len(docDOs))
	for _, d := range docDOs {
		docToName[d.ID] = d.FileName
	}

	// 富化每个 chunk 的 Metadata
	for i := range chunks {
		docID, ok := chunkToDoc[chunks[i].ID]
		if !ok {
			continue
		}
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]any)
		}
		chunks[i].Metadata["doc_id"] = docID
		if name, ok2 := docToName[docID]; ok2 {
			chunks[i].Metadata["doc_name"] = name
			chunks[i].Metadata["file_name"] = name
		}
	}

	return chunks, nil
}
```

- [ ] **Step 2: 验证编译**（这时 main.go 还没接线，但包编译应通过）

```bash
cd goRAGENT && go build ./internal/rag/retrieve/postprocessor/...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/internal/rag/retrieve/postprocessor/metadata_enrich.go
git commit -m "feat(m4): add metadata enrichment post-processor (docId/docName from MySQL)"
```

---

### Task 16: main.go 接线 — 装配所有 M4 组件

**Files:**
- Modify: `goRAGENT/cmd/server/main.go`

**Interfaces:**
- Consumes: All M4 components from Tasks 2-15
- Produces: 完整的启动链路

- [ ] **Step 1: 在 main.go 中装配 mineru + ingestion + 新 admin 依赖**

修改 `main.go`：

**加 import：**
```go
"github.com/nageoffer/ragent/goRAGENT/internal/infra/mineru"
"github.com/nageoffer/ragent/goRAGENT/internal/ingestion"
"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve/postprocessor"
```

**在 `embedSvc` 之后加 mineru + ingestion engine 装配**（约第81行之后）：
```go
// M4: MinerU + 入库引擎
mineruClient := mineru.NewClient(cfg.Mineru.APIToken)
var ingestionEngine *ingestion.Engine
if mvStore != nil {
    ingestionEngine = ingestion.NewEngine(db, mineruClient, embedSvc, mvStore, cfg.Ingestion)
}
```

**修改 admin handler 初始化**，追加 setter：
```go
adminH := admin.NewHandler(db).
    SetIntentCacheClearer(intentLoader).
    SetMappingCacheClearer(mappingLoader).
    SetMilvusStore(mvStore).
    SetIngestionEngine(ingestionEngine).
    SetDataDir(cfg.Mineru.DataDir)
```

**修改 postProcessors 列表**，在前面插入 MetadataEnrichment：
```go
postProcessors := []retrieve.PostProcessor{
    postprocessor.NewMetadataEnrichmentPostProcessor(db),  // M4: order=0
    &retrieve.DedupPostProcessor{},
    &retrieve.FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 50},
    retrieve.NewRerankPostProcessor(retrieve.RerankerAdapter(rerankSvc), cfg.RAG.RerankEnabled),
}
```

**更新空壳路由**，用 `adminH` 替换 `admin.NewHandler(db)` 调用（main.go 第150-155行）：
```go
api.GET("/knowledge-base", adminH.ListKnowledgeBases)
api.POST("/knowledge-base", adminH.CreateKnowledgeBase)
api.GET("/knowledge-base/:id", adminH.GetKnowledgeBase)
api.PUT("/knowledge-base/:id", adminH.UpdateKnowledgeBase)
api.DELETE("/knowledge-base/:id", adminH.DeleteKnowledgeBase)
api.GET("/knowledge-base/docs/search", adminH.SearchDocuments)
```

**补 main.go 中缺失的路由**（文档/Chunk/Ingestion）用 `adminH` 替换空壳：
```go
// 把现有 main.go 中所有 admin.NewHandler(db).xxx 替换为 adminH.xxx
```

具体需要替换的行（已在 main.go 中定义了但返回假数据的）：
- `api.GET("/knowledge-base/docs/:docId/chunks/:chunkId"...)` → `adminH.GetChunk`
- `api.PUT("/knowledge-base/docs/:docId/chunks/:chunkId"...)` → `adminH.UpdateChunk`
- `api.PATCH("/knowledge-base/docs/:docId/chunks/:chunkId/enable"...)` → `adminH.ToggleChunk`
- `api.PATCH("/knowledge-base/docs/:docId/enable"...)` → `adminH.ToggleDocument`
- `api.POST("/knowledge-base/docs/:docId/chunk"...)` → `adminH.CreateChunk`
- ingestion routes → `adminH.ListIngestionTasks` / `adminH.GetIngestionTask` / `adminH.GetIngestionTaskNodes`

- [ ] **Step 2: 验证编译**

```bash
cd goRAGENT && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add goRAGENT/cmd/server/main.go
git commit -m "feat(m4): wire MinerU + ingestion engine + metadata enrichment in main.go"
```

---

### Task 17: 集成验证

**Files:**
- No code changes — verification only

- [ ] **Step 1: 确保所有编译通过**

```bash
cd goRAGENT && go build ./...
```
Expected: 无错误

- [ ] **Step 2: 确保已有测试不退化**

```bash
cd goRAGENT && go test ./... -count=1
```
Expected: 102 PASS（和 handover.md 基准一致，无新增失败）

- [ ] **Step 3: 验证知识库 CRUD 端点**

```bash
# 启动服务
cd goRAGENT && go run ./cmd/server/ &

# 创建知识库（需要 JWT token，先用登录获取）
TOKEN=$(curl -s -X POST http://localhost:9090/api/ragent/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"intenttest","password":"test123456"}' | jq -r '.data.token')

# 创建知识库
curl -s -X POST http://localhost:9090/api/ragent/knowledge-base \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"测试知识库","description":"M4 集成测试"}'
# Expected: {"code":"0","data":{"id":"...","name":"测试知识库",...}}

# 列表
curl -s http://localhost:9090/api/ragent/knowledge-base \
  -H "Authorization: Bearer $TOKEN"
# Expected: {"code":"0","data":{"total":1,"rows":[...]}}
```

- [ ] **Step 4: 验证文件上传 + 入库流程**

```bash
# 上传测试 Markdown 文件
echo "# 测试文档\n\n这是一个测试段落。\n\n## 第二节\n\n更多测试内容。" > /tmp/test-m4.md

KB_ID=$(curl -s http://localhost:9090/api/ragent/knowledge-base \
  -H "Authorization: Bearer $TOKEN" | jq -r '.data.rows[0].id')

curl -s -X POST "http://localhost:9090/api/ragent/knowledge-base/$KB_ID/docs/upload" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/test-m4.md"
# Expected: {"code":"0","data":{"id":"...","status":"PENDING",...}}

# 等几秒后查文档状态
sleep 5
# 查入库任务
curl -s http://localhost:9090/api/ragent/ingestion/tasks \
  -H "Authorization: Bearer $TOKEN"
# Expected: 有任务记录，status 应为 DONE
```

- [ ] **Step 5: 验证 Milvus Collection 存在**

```bash
# 通过 Attu 或代码日志确认 Milvus Collection kb_{id} 已创建且有数据
```

- [ ] **Step 6: 验证问答能检索到入库内容**

```bash
curl -s "http://localhost:9090/api/ragent/rag/v3/chat?question=测试文档" \
  -H "Authorization: Bearer $TOKEN"
# Expected: SSE 流中 message 内容引用了测试文档的内容（不再走空检索短路）
```

- [ ] **Step 7: Commit 验证结果（如果有微调）**

```bash
git add -A && git diff --cached --stat
git commit -m "chore(m4): integration verification tweaks"
```
