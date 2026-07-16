package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/infra/embedding"
	"goRAGENT/internal/rag"
	"goRAGENT/internal/rag/retrieve/vectorstore"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Indexer 嵌入+索引+落库节点
type Indexer struct {
	db        *gorm.DB
	embed     *embedding.Service
	milvus    *vectorstore.MilvusStore
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

	texts := make([]string, len(pc.Chunks))
	for i, c := range pc.Chunks {
		texts[i] = c.Text
	}

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

		// 2. 构建 Milvus + MySQL 数据
		milvusChunks := make([]vectorstore.ChunkVector, len(batchChunks))
		mysqlChunks := make([]rag.ChunkDO, len(batchChunks))

		for i, chunk := range batchChunks {
			chunkID := snowflake.NextID()
			meta, _ := json.Marshal(map[string]any{
				"doc_id":      pc.Doc.ID,
				"kb_id":       pc.Doc.KbID,
				"file_name":   pc.Doc.FileName,
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

	// 更新 t_document
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
