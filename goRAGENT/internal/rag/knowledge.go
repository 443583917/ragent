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

// 入库相关状态常量
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
