package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
	"go.uber.org/zap"
)

type MilvusStore struct {
	client   client.Client
	embedder Embedder
}

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

func NewMilvusStore(uri string, embedder Embedder) (*MilvusStore, error) {
	c, err := client.NewClient(context.Background(), client.Config{Address: uri})
	if err != nil {
		return nil, fmt.Errorf("Milvus 连接失败: %w", err)
	}
	return &MilvusStore{client: c, embedder: embedder}, nil
}

func (m *MilvusStore) Search(ctx context.Context, collection string, query string, topK int) ([]retrieve.RetrievedChunk, error) {
	if err := m.client.LoadCollection(ctx, collection, false); err != nil {
		return nil, fmt.Errorf("加载 collection 失败: %w", err)
	}
	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("向量化失败: %w", err)
	}
	sp, _ := entity.NewIndexFlatSearchParam()
	result, err := m.client.Search(ctx, collection, []string{}, "",
		[]string{"id", "text", "metadata"},
		[]entity.Vector{entity.FloatVector(vec)},
		"vector", entity.COSINE, topK, sp,
	)
	if err != nil {
		return nil, fmt.Errorf("检索失败: %w", err)
	}
	var chunks []retrieve.RetrievedChunk
	for _, r := range result {
		idCol := r.Fields[0].(*entity.ColumnVarChar)
		textCol := r.Fields[1].(*entity.ColumnVarChar)
		metaCol := r.Fields[2].(*entity.ColumnVarChar)
		for i := 0; i < r.ResultCount; i++ {
			id, _ := idCol.ValueByIdx(i)
			text, _ := textCol.ValueByIdx(i)
			metaStr, _ := metaCol.ValueByIdx(i)
			var meta map[string]any
			json.Unmarshal([]byte(metaStr), &meta)
			chunks = append(chunks, retrieve.RetrievedChunk{
				ID: id, Text: text, Score: float64(r.Scores[i]), Metadata: meta,
			})
		}
	}
	zap.L().Debug("Milvus done", zap.String("col", collection), zap.Int("n", len(chunks)))
	return chunks, nil
}

func (m *MilvusStore) ListCollections(ctx context.Context) ([]string, error) {
	cols, err := m.client.ListCollections(ctx)
	if err != nil { return nil, err }
	var names []string
	for _, c := range cols { names = append(names, c.Name) }
	return names, nil
}
