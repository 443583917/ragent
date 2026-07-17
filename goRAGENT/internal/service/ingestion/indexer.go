package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"goRAGENT/pkg/snowflake"
	"goRAGENT/pkg/embedding"
	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/milvus"
	"go.uber.org/zap"
)

// Indexer 嵌入+索引+落库节点
type Indexer struct {
	chunkRepo repository.ChunkRepository
	taskRepo  repository.IngestionTaskRepository
	docRepo   repository.DocumentRepository
	embed     *embedding.Service
	milvus    *milvus.MilvusStore
	batchSize int
}

func NewIndexer(chunkRepo repository.ChunkRepository, taskRepo repository.IngestionTaskRepository,
	docRepo repository.DocumentRepository, embedSvc *embedding.Service,
	milvusStore *milvus.MilvusStore, batchSize int) *Indexer {
	return &Indexer{chunkRepo: chunkRepo, taskRepo: taskRepo, docRepo: docRepo,
		embed: embedSvc, milvus: milvusStore, batchSize: batchSize}
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
		milvusChunks := make([]milvus.ChunkVector, len(batchChunks))
		mysqlChunks := make([]model.ChunkDO, len(batchChunks))

		for i, chunk := range batchChunks {
			chunkID := snowflake.NextID()
			meta, _ := json.Marshal(map[string]any{
				"doc_id":      pc.Doc.ID,
				"kb_id":       pc.Doc.KbID,
				"file_name":   pc.Doc.FileName,
				"chunk_index": chunk.Index,
			})

			milvusChunks[i] = milvus.ChunkVector{
				ID:       chunkID,
				Text:     chunk.Text,
				Metadata: string(meta),
				Vector:   vectors[i],
			}

			mysqlChunks[i] = model.ChunkDO{
				ID:              chunkID,
				DocID:           pc.Doc.ID,
				KbID:            pc.Doc.KbID,
				ChunkIndex:      chunk.Index,
				Text:            chunk.Text,
				CharCount:       chunk.CharCount,
				EmbeddingStatus: model.EmbedStatusDone,
				Enabled:         1,
			}
		}

		// 3. Milvus Insert
		if err := idx.milvus.Insert(ctx, pc.KB.CollectionName, milvusChunks); err != nil {
			return fmt.Errorf("Milvus Insert 失败 (offset=%d): %w", start, err)
		}

		// 4. MySQL 批量写入
		if err := idx.chunkRepo.BatchCreate(ctx, mysqlChunks); err != nil {
			return fmt.Errorf("MySQL 写入 t_chunk 失败 (offset=%d): %w", start, err)
		}

		// 5. 更新 task 进度
		if err := idx.taskRepo.UpdateFields(ctx, strconv.FormatInt(pc.Task.ID, 10),
			map[string]any{"completed_chunks": end}); err != nil {
			return fmt.Errorf("更新 task 进度失败 (offset=%d): %w", start, err)
		}
	}

	// 更新 t_document
	if err := idx.docRepo.UpdateFields(ctx, pc.Doc.ID, map[string]any{
		"chunk_count": totalChunks,
		"status":      model.DocStatusDone,
	}); err != nil {
		return fmt.Errorf("更新文档状态失败: %w", err)
	}

	zap.L().Info("Indexer 入库完成",
		zap.String("doc_id", pc.Doc.ID),
		zap.Int("total_chunks", totalChunks),
	)
	return nil
}
