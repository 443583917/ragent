package admin

import (
	"context"
	"errors"
	"testing"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// mockKBRepo 手写 Mock KnowledgeBaseRepository
type mockKBRepo struct {
	repository.KnowledgeBaseRepository
	listFn       func(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error)
	findByIDFn   func(ctx context.Context, id string) (*model.KnowledgeBaseDO, error)
	createFn     func(ctx context.Context, kb *model.KnowledgeBaseDO) error
	updateFields func(ctx context.Context, id string, updates map[string]any) error
	softDeleteFn func(ctx context.Context, id string) error
}

func (m *mockKBRepo) List(ctx context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error) {
	return m.listFn(ctx, q)
}
func (m *mockKBRepo) FindByID(ctx context.Context, id string) (*model.KnowledgeBaseDO, error) {
	return m.findByIDFn(ctx, id)
}
func (m *mockKBRepo) Create(ctx context.Context, kb *model.KnowledgeBaseDO) error {
	return m.createFn(ctx, kb)
}
func (m *mockKBRepo) SoftDelete(ctx context.Context, id string) error {
	return m.softDeleteFn(ctx, id)
}

// mockVectorStore 手写 Mock VectorStore
type mockVectorStore struct {
	createCollectionFn func(ctx context.Context, name string, dimension int) error
	dropCollectionFn   func(ctx context.Context, name string) error
}

func (m *mockVectorStore) CreateCollection(ctx context.Context, name string, dimension int) error {
	return m.createCollectionFn(ctx, name, dimension)
}
func (m *mockVectorStore) DropCollection(ctx context.Context, name string) error {
	return m.dropCollectionFn(ctx, name)
}

func TestKnowledgeService_List(t *testing.T) {
	repo := &mockKBRepo{
		listFn: func(_ context.Context, q model.PageQuery) ([]model.KnowledgeBaseDO, int64, error) {
			if q.Page != 1 || q.Size != 20 {
				t.Errorf("unexpected page query: %+v", q)
			}
			return nil, 0, nil
		},
	}
	svc := NewKnowledgeBaseService(repo, nil)
	vos, total, err := svc.List(context.Background(), model.PageQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(vos) != 0 {
		t.Errorf("expected empty result, got total=%d vos=%d", total, len(vos))
	}
}

func TestKnowledgeService_Create_VectorStoreError(t *testing.T) {
	createCalled := false
	vs := &mockVectorStore{
		createCollectionFn: func(_ context.Context, _ string, _ int) error {
			return errors.New("milvus connection refused")
		},
	}
	repo := &mockKBRepo{
		createFn: func(_ context.Context, _ *model.KnowledgeBaseDO) error {
			createCalled = true
			return nil
		},
	}
	svc := NewKnowledgeBaseService(repo, vs)
	_, err := svc.Create(context.Background(), model.KnowledgeBaseCreateReq{Name: "test"})
	if err == nil {
		t.Fatal("expected error when VectorStore.CreateCollection fails")
	}
	if createCalled {
		t.Error("should not call repo.Create when VectorStore fails")
	}
}

func TestKnowledgeService_Create_Success(t *testing.T) {
	vs := &mockVectorStore{
		createCollectionFn: func(_ context.Context, _ string, _ int) error {
			return nil
		},
	}
	repo := &mockKBRepo{
		createFn: func(_ context.Context, kb *model.KnowledgeBaseDO) error {
			if kb.Name != "test-kb" {
				t.Errorf("unexpected name: %s", kb.Name)
			}
			return nil
		},
	}
	svc := NewKnowledgeBaseService(repo, vs)
	vo, err := svc.Create(context.Background(), model.KnowledgeBaseCreateReq{Name: "test-kb"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vo.Name != "test-kb" {
		t.Errorf("expected name 'test-kb', got %q", vo.Name)
	}
	if vo.CollectionName == "" {
		t.Error("collection name should not be empty")
	}
}

func TestKnowledgeService_Delete_VectorStoreNil(t *testing.T) {
	repo := &mockKBRepo{
		findByIDFn: func(_ context.Context, id string) (*model.KnowledgeBaseDO, error) {
			return &model.KnowledgeBaseDO{ID: id, CollectionName: "kb_test"}, nil
		},
		softDeleteFn: func(_ context.Context, id string) error {
			return nil
		},
	}
	// When vectorStore is nil, Delete should not panic
	svc := NewKnowledgeBaseService(repo, nil)
	if err := svc.Delete(context.Background(), "test-id"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeService_Delete_NotFound(t *testing.T) {
	repo := &mockKBRepo{
		findByIDFn: func(_ context.Context, _ string) (*model.KnowledgeBaseDO, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewKnowledgeBaseService(repo, nil)
	err := svc.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent KB")
	}
}

func TestKnowledgeService_Get_NotFound(t *testing.T) {
	repo := &mockKBRepo{
		findByIDFn: func(_ context.Context, _ string) (*model.KnowledgeBaseDO, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewKnowledgeBaseService(repo, nil)
	_, err := svc.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent KB")
	}
}
