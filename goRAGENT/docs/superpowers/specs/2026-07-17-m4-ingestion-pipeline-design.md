# M4 文档入库 Pipeline 设计文档

> 日期: 2026-07-17 | 状态: draft | 关联: development-tasks.md M4 章节

## 一、目标

实装知识库/文档/Chunk 管理 CRUD + 文档入库 Pipeline（解析→分块→嵌入→Milvus 索引）+ 检索元数据富化后处理器。M4 完成后，问答可检索到入库文档的真实内容（不再走空检索短路）。

## 二、数据模型

### 2.1 已有表（改 ID 类型）

- `t_knowledge_base` — 已存在，**id 改为 VARCHAR(32) snowflake**，其余字段不变
- `t_document` — 已存在，**id 改为 VARCHAR(32) snowflake**，其余字段不变

> 理由：`t_intent_node`、`t_conversation` 等表已统一使用 snowflake VARCHAR(32) ID，KB/Doc 跟随此模式。

### 2.2 新增表

**t_chunk**（需加到 init.sql）：

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
```

**t_ingestion_task**（需加到 init.sql）：

```sql
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

### 2.3 Go DO 模型（新文件 `internal/rag/knowledge.go`）

```go
// 所有 ID 统一使用 snowflake VARCHAR(32)，和 IntentNodeDO / ConversationDO 一致

// KnowledgeBaseDO t_knowledge_base
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

// DocumentDO t_document
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

// ChunkDO t_chunk
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

// IngestionTaskDO t_ingestion_task
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
```

## 三、Milvus 写操作（补 `internal/rag/retrieve/vectorstore/milvus.go`）

新增 4 个方法，保持和现有 `Search`/`ListCollections` 一致风格：

| 方法 | 说明 |
|------|------|
| `HasCollection(ctx, name) (bool, error)` | 检查 Collection 是否存在 |
| `CreateCollection(ctx, name string, dim int) error` | 建 Collection，COSINE 度量，Flat index；字段 id(VarChar 128), text(VarChar 65535), metadata(VarChar 65535), vector(FloatVector) |
| `Insert(ctx, collection string, chunks []ChunkVector) error` | 批量插入向量数据，每个 chunk 含 ID/Text/Metadata/Vector |
| `DropCollection(ctx, name) error` | 删除 Collection |

新增结构体：
```go
type ChunkVector struct {
    ID       string
    Text     string
    Metadata string // JSON string
    Vector   []float32
}
```

## 四、入库 Pipeline（新包 `internal/ingestion/`）

### 4.1 包结构

```
internal/ingestion/
  node.go        # Node 接口 + PipelineContext
  engine.go      # Engine 异步执行节点链 + task 状态管理
  fetcher.go     # Fetcher: 读本地文件
  parser.go      # Parser: mineru.Client.Parse() 或 .md/.txt 直读
  chunker.go     # Chunker: Markdown 标题混合切分 (对齐 Java)
  indexer.go     # Indexer: embedBatch → Milvus Insert + t_chunk 落库
```

### 4.2 核心接口

```go
type Node interface {
    Name() string
    Execute(ctx context.Context, pc *PipelineContext) error
}

type PipelineContext struct {
    Task      *IngestionTaskDO
    KB        *KnowledgeBaseDO
    Doc       *DocumentDO
    FilePath  string           // fetcher 产出文件路径
    Markdown  string           // parser 产出
    Chunks    []ChunkSegment   // chunker 产出
}

type ChunkSegment struct {
    Index     int
    Text      string
    CharCount int
}
```

### 4.3 节点执行流

```
Fetcher → Parser → Chunker → Indexer
   ↓         ↓         ↓         ↓
 读文件    MinerU   混合切分   Embed+Insert
 验证存在  .md直读  标题+大小  Milvus+MySQL
```

- **Fetcher**: 按 doc.kb_id + doc.id 拼接文件路径 `data/files/{kb_id}/{doc_id}.{ext}`，读入 `pc.FilePath`，校验文件存在
- **Parser**: `.md`/`.txt`/`.markdown` 直接 `os.ReadFile`；其他格式调 `mineru.Client.Parse(ctx, pc.FilePath)` 返回 markdown 文本
- **Chunker**: Markdown 标题混合切分——先按 `#` 标题分割 section，超过阈值（512 token，即约 1024 字符）的 section 按固定大小（512 char）递归切分，带 50 char 重叠。输出 `[]ChunkSegment`
- **Indexer**: 1) 批量 Embedding（`embedding.EmbedBatch`，batch size 32）；2) Milvus Insert 带 metadata（doc_id, kb_id, chunk_index, file_name）；3) `t_chunk` 批量写入 MySQL；4) 更新 `t_document.chunk_count` 和 `t_ingestion_task.status`

### 4.4 引擎 + 异步模型

```go
type Engine struct {
    db      *gorm.DB
    mineru  *mineru.Client
    embed   *embedding.Service
    milvus  *vectorstore.MilvusStore
}

// Run 异步执行 Pipeline（go engine.Run(taskID)）
func (e *Engine) Run(taskID int64) {
    // 1. 查 task → 查 doc → 查 kb
    // 2. task.status = RUNNING
    // 3. 顺序执行 Fetcher→Parser→Chunker→Indexer
    // 4. 任一步失败 → task.status = FAILED, task.error_message = err.Error()
    // 5. 全部成功 → task.status = DONE
}
```

## 五、Admin CRUD 实现

### 5.1 知识库（5 端点，覆盖 admin.go 空壳）

| 端点 | 逻辑 |
|------|------|
| `GET /knowledge-base` | 分页查询 `WHERE deleted=0`，返回列表 VO |
| `POST /knowledge-base` | 校验 name 必填 → 生成 CollectionName = `kb_{snowflake_id}` → 写 MySQL → `milvus.CreateCollection(collectionName, 1536)` → 返回 id |
| `GET /knowledge-base/:id` | 按 id 查单条，返回详情 VO |
| `PUT /knowledge-base/:id` | 部分更新 name/description（collection_name 不可改） |
| `DELETE /knowledge-base/:id` | 软删 MySQL → `milvus.DropCollection(collectionName)` |

### 5.2 文档（7 端点）

| 端点 | 逻辑 |
|------|------|
| `GET /kb/:id/docs` | 按 kb_id 分页查文档列表 |
| `POST /kb/:id/docs/upload` | multipart form → `data/files/{kb_id}/{doc_id}.{ext}` 落盘 → 写 `t_document`(status=PENDING) → 建 task(PENDING) → `go engine.Run(task.ID)` |
| `GET /kb/docs/search` | 按 keyword 搜索文档名 |
| `GET /kb/docs/:id` | 文档详情 |
| `GET /kb/docs/:id/preview` | 读 `data/parsed/{doc_id}.md` 返回前 5000 字符 |
| `GET /kb/docs/:id/file` | 文件下载（Content-Disposition attachment） |
| `PATCH /kb/docs/:docId/enable` | 启停文档 |

### 5.3 文档删除（补充端点 `DELETE /kb/docs/:id`）

软删文档 + 关联 chunk 软删 + 清理文件 `data/files/` 和 `data/parsed/`

### 5.4 Chunk（4 端点）

| 端点 | 逻辑 |
|------|------|
| `GET /kb/:id/chunks` | 按 kb_id 分页 |
| `GET /datasets/:id/chunks` | 按 doc_id 分页 |
| `GET /kb/docs/:docId/chunks/:chunkId` | chunk 详情 |
| `PUT /kb/docs/:docId/chunks/:chunkId` | 更新 chunk text |
| `PATCH /kb/docs/:docId/chunks/:chunkId/enable` | 启停 chunk |

### 5.5 入库任务（6 端点）

| 端点 | 逻辑 |
|------|------|
| `GET /ingestion/tasks` | 分页列表 |
| `GET /ingestion/tasks/:id` | 任务详情（含状态/进度） |
| `POST /ingestion/tasks/upload` | 同文档上传入口 |
| `GET /ingestion/tasks/:id/nodes` | 返回节点状态（预留，M4 返回固定 4 节点） |

## 六、文件存储

统一目录：`data/`（在 goRAGENT 工作目录下）

```
data/
  files/        # 上传原始文件  {kb_id}/{doc_id}.{ext}
  parsed/       # MinerU 解析产物 {doc_id}.md
```

- 上传时：`data/files/{kb_id}/{doc_id}.{ext}`
- 解析后：`data/parsed/{doc_id}.md`
- 文档删除时同步清理两个目录
- MinerU 的所有中间产物都在 data 下进行

## 七、检索元数据富化后处理器（M4-6）

新文件 `internal/rag/retrieve/postprocessor/metadata_enrich.go`：

```go
type MetadataEnrichmentPostProcessor struct {
    db *gorm.DB
}

func (p *MetadataEnrichmentPostProcessor) Name() string { return "MetadataEnrichment" }
func (p *MetadataEnrichmentPostProcessor) Order() int    { return 0 } // 最先执行

func (p *MetadataEnrichmentPostProcessor) Process(ctx context.Context, chunks []RetrievedChunk, sc *SearchContext) ([]RetrievedChunk, error) {
    // 从 chunks 的 metadata.chunk_id 收集所有 chunk_id
    // 回表 t_chunk JOIN t_document 查 doc_id → doc_name（批量 IN 查询）
    // 将 docId/docName 富化到每个 chunk 的 Metadata 上
}
```

现有后处理器执行顺序调整为：MetadataEnrichment(0) → Dedup(1) → Fusion(5) → Rerank(10)。

## 八、配置扩展

`MineruConfig` 已存在，新增 2 个配置项：

```go
type MineruConfig struct {
    APIToken string
    DataDir  string // 文件管理目录，默认 "data"
}
type IngestionConfig struct {
    ChunkSize      int  // 分块大小（字符），默认 1024
    ChunkOverlap   int  // 重叠大小（字符），默认 50
    EmbedBatchSize int  // 嵌入批大小，默认 32
}
```

环境变量：
```
MINERU_DATA_DIR=data
INGESTION_CHUNK_SIZE=1024
INGESTION_CHUNK_OVERLAP=50
INGESTION_EMBED_BATCH_SIZE=32
```

## 九、main.go 接线

```go
// 新增 import
mineruClient := mineru.NewClient(cfg.Mineru.APIToken)
ingestionEngine := ingestion.NewEngine(db, mineruClient, embedSvc, mvStore)

// Admin handler 注入新依赖
adminH := admin.NewHandler(db).
    SetIntentCacheClearer(intentLoader).
    SetMappingCacheClearer(mappingLoader).
    SetIngestionEngine(ingestionEngine).
    SetDataDir(cfg.Mineru.DataDir)

// 检索后处理器加 MetadataEnrichment
postProcessors := []retrieve.PostProcessor{
    retrieve.NewMetadataEnrichmentPostProcessor(db),  // order=0, 最先
    &retrieve.DedupPostProcessor{},
    ...
}
```

## 十、文件清单

| 文件 | 操作 | 说明 |
|------|:--:|------|
| `docker/init.sql` | 改 | 加 t_chunk + t_ingestion_task |
| `internal/rag/knowledge.go` | 新 | 4 个 DO 模型 |
| `internal/rag/retrieve/vectorstore/milvus.go` | 改 | 加 HasCollection/CreateCollection/Insert/DropCollection |
| `internal/ingestion/node.go` | 新 | Node 接口 + PipelineContext |
| `internal/ingestion/engine.go` | 新 | Engine + Run 异步编排 |
| `internal/ingestion/fetcher.go` | 新 | Fetcher 节点 |
| `internal/ingestion/parser.go` | 新 | Parser 节点 |
| `internal/ingestion/chunker.go` | 新 | Chunker 节点（混合切分） |
| `internal/ingestion/indexer.go` | 新 | Indexer 节点 |
| `internal/admin/knowledge_base.go` | 新 | 知识库 CRUD 实装 |
| `internal/admin/document.go` | 新 | 文档 CRUD + 上传 + 预览/下载 |
| `internal/admin/chunk.go` | 新 | Chunk CRUD |
| `internal/admin/ingestion_task.go` | 新 | 入库任务监控 API |
| `internal/rag/retrieve/postprocessor/metadata_enrich.go` | 新 | 元数据富化后处理器 |
| `internal/config/config.go` | 改 | 加 IngestionConfig |
| `cmd/server/main.go` | 改 | 装配 mineru/ingestion/新 admin 依赖 |
| `internal/admin/admin.go` | 改 | Handler 加 ingestion + dataDir 字段，路由指向真实 handler |
