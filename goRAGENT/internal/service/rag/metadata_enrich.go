package rag

import (
	"context"

	"goRAGENT/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type MetadataEnrichmentPostProcessor struct {
	db *gorm.DB
}

func NewMetadataEnrichmentPostProcessor(db *gorm.DB) *MetadataEnrichmentPostProcessor {
	return &MetadataEnrichmentPostProcessor{db: db}
}

func (p *MetadataEnrichmentPostProcessor) Name() string { return "MetadataEnrichment" }
func (p *MetadataEnrichmentPostProcessor) Order() int   { return 0 }

func (p *MetadataEnrichmentPostProcessor) IsEnabled(ctx context.Context, sc *model.SearchContext) bool {
	return true
}

func (p *MetadataEnrichmentPostProcessor) Process(ctx context.Context, chunks []model.RetrievedChunk, sc *model.SearchContext) ([]model.RetrievedChunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}

	chunkIDs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.ID != "" {
			chunkIDs = append(chunkIDs, c.ID)
		}
	}
	if len(chunkIDs) == 0 {
		return chunks, nil
	}

	var chunkDOs []model.ChunkDO
	if err := p.db.WithContext(ctx).Select("id, doc_id").
		Where("id IN ? AND deleted = 0", chunkIDs).Find(&chunkDOs).Error; err != nil {
		zap.L().Warn("MetadataEnrichment 查 chunk 失败", zap.Error(err))
		return chunks, nil
	}

	chunkToDoc := make(map[string]string, len(chunkDOs))
	docIDSet := make(map[string]bool)
	for _, c := range chunkDOs {
		chunkToDoc[c.ID] = c.DocID
		docIDSet[c.DocID] = true
	}

	docIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		docIDs = append(docIDs, id)
	}

	var docDOs []model.DocumentDO
	if len(docIDs) > 0 {
		p.db.WithContext(ctx).Select("id, file_name").
			Where("id IN ? AND deleted = 0", docIDs).Find(&docDOs)
	}

	docToName := make(map[string]string, len(docDOs))
	for _, d := range docDOs {
		docToName[d.ID] = d.FileName
	}

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
