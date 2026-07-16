package mysql

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"goRAGENT/internal/model"
)

// newTestDB 打开内存 sqlite 并建表。
func newTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存 sqlite 失败: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	return db
}

func TestKnowledgeBaseRepo_Create_FindByID(t *testing.T) {
	db := newTestDB(t, &model.KnowledgeBaseDO{})
	repo := NewKnowledgeBaseRepo(db)
	ctx := context.Background()

	kb := model.KnowledgeBaseDO{ID: "kb1", Name: "测试库", CollectionName: "kb_kb1", Dimension: 1536}
	if err := repo.Create(ctx, &kb); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	got, err := repo.FindByID(ctx, "kb1")
	if err != nil {
		t.Fatalf("FindByID 失败: %v", err)
	}
	if got.Name != "测试库" || got.CollectionName != "kb_kb1" {
		t.Fatalf("FindByID 返回不一致: %+v", got)
	}
}

func TestKnowledgeBaseRepo_List_Pagination(t *testing.T) {
	db := newTestDB(t, &model.KnowledgeBaseDO{})
	repo := NewKnowledgeBaseRepo(db)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := repo.Create(ctx, &model.KnowledgeBaseDO{ID: id, Name: "kb-" + id}); err != nil {
			t.Fatalf("Create %s 失败: %v", id, err)
		}
	}

	cases := []struct {
		name      string
		q         model.PageQuery
		wantRows  int
		wantTotal int64
	}{
		{name: "第一页两条", q: model.PageQuery{Page: 1, Size: 2}, wantRows: 2, wantTotal: 3},
		{name: "第二页一条", q: model.PageQuery{Page: 2, Size: 2}, wantRows: 1, wantTotal: 3},
		{name: "非法参数归一化不 panic", q: model.PageQuery{Page: 0, Size: 0}, wantRows: 3, wantTotal: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := repo.List(ctx, tc.q)
			if err != nil {
				t.Fatalf("List 失败: %v", err)
			}
			if len(rows) != tc.wantRows || total != tc.wantTotal {
				t.Fatalf("List 期望 rows=%d total=%d, 实际 rows=%d total=%d",
					tc.wantRows, tc.wantTotal, len(rows), total)
			}
		})
	}
}

func TestKnowledgeBaseRepo_UpdateFields_FieldsChanged(t *testing.T) {
	db := newTestDB(t, &model.KnowledgeBaseDO{})
	repo := NewKnowledgeBaseRepo(db)
	ctx := context.Background()

	if err := repo.Create(ctx, &model.KnowledgeBaseDO{ID: "kb1", Name: "旧名"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if err := repo.UpdateFields(ctx, "kb1", map[string]any{"name": "新名", "description": "描述"}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	got, err := repo.FindByID(ctx, "kb1")
	if err != nil {
		t.Fatalf("FindByID 失败: %v", err)
	}
	if got.Name != "新名" || got.Description != "描述" {
		t.Fatalf("字段未更新: %+v", got)
	}

	// 空 updates 应为 no-op
	if err := repo.UpdateFields(ctx, "kb1", map[string]any{}); err != nil {
		t.Fatalf("空 updates 应无错误: %v", err)
	}
}

func TestKnowledgeBaseRepo_SoftDelete_FindByIDFails(t *testing.T) {
	db := newTestDB(t, &model.KnowledgeBaseDO{})
	repo := NewKnowledgeBaseRepo(db)
	ctx := context.Background()

	if err := repo.Create(ctx, &model.KnowledgeBaseDO{ID: "kb1", Name: "待删"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if err := repo.SoftDelete(ctx, "kb1"); err != nil {
		t.Fatalf("SoftDelete 失败: %v", err)
	}
	if _, err := repo.FindByID(ctx, "kb1"); err == nil {
		t.Fatal("软删除后 FindByID 应返回错误")
	}
	// 列表也不应再包含
	rows, total, err := repo.List(ctx, model.PageQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Fatalf("软删除后 List 应为空, 实际 rows=%d total=%d", len(rows), total)
	}
}
