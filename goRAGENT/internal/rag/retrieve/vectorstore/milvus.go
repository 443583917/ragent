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

// ChunkVector 待入库的向量化文档块
type ChunkVector struct {
	ID       string
	Text     string
	Metadata string // JSON string
	Vector   []float32
}

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

// HasCollection 检查 Collection 是否存在
func (m *MilvusStore) HasCollection(ctx context.Context, name string) (bool, error) {
	return m.client.HasCollection(ctx, name)
}

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

	schema := entity.NewSchema().WithName(name).
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dim)))

	if err := m.client.CreateCollection(ctx, schema, int32(2)); err != nil {
		return fmt.Errorf("创建 collection 失败: %w", err)
	}

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

// Insert 批量插入向量数据
func (m *MilvusStore) Insert(ctx context.Context, collection string, chunks []ChunkVector) error {
	if len(chunks) == 0 {
		return nil
	}

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

	vecDim := 0
	if len(vecCol) > 0 {
		vecDim = len(vecCol[0])
	}
	columns := []entity.Column{
		entity.NewColumnVarChar("id", idCol),
		entity.NewColumnVarChar("text", textCol),
		entity.NewColumnVarChar("metadata", metaCol),
		entity.NewColumnFloatVector("vector", vecDim, vecCol),
	}

	if _, err := m.client.Insert(ctx, collection, "", columns...); err != nil {
		return fmt.Errorf("Milvus Insert 失败: %w", err)
	}

	if err := m.client.Flush(ctx, collection, false); err != nil {
		zap.L().Warn("Milvus Flush 失败", zap.Error(err))
	}

	zap.L().Info("Milvus Insert 完成", zap.String("collection", collection), zap.Int("count", len(chunks)))
	return nil
}

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
