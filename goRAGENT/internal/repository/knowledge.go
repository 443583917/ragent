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
	// UpdateFieldsByDoc 按 doc_id 批量更新未删除分块（toggleDocument 语义需要，接口定义补充项）。
	UpdateFieldsByDoc(ctx context.Context, docID string, updates map[string]any) error
	SoftDeleteByDoc(ctx context.Context, docID string) error
}

// IngestionTaskRepository 入库任务表数据访问。
type IngestionTaskRepository interface {
	List(ctx context.Context, q model.PageQuery) ([]model.IngestionTaskDO, int64, error)
	FindByID(ctx context.Context, id string) (*model.IngestionTaskDO, error)
	Create(ctx context.Context, t *model.IngestionTaskDO) error
	UpdateFields(ctx context.Context, id string, updates map[string]any) error
}
