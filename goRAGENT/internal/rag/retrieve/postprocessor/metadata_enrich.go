package postprocessor

import (
	"context"

	"goRAGENT/internal/rag"
	"goRAGENT/internal/rag/retrieve"
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
		return chunks, nil
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
