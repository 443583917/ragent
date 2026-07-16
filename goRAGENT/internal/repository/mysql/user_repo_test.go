package mysql

import (
	"context"
	"strconv"
	"testing"

	"goRAGENT/internal/model"
)

func TestUserRepo_Create_FindByUsername(t *testing.T) {
	db := newTestDB(t, &model.UserDO{})
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := model.UserDO{Username: "alice", Password: "hash", Role: model.RoleUser}
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("Create 后应回填自增 ID")
	}

	got, err := repo.FindByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByUsername 失败: %v", err)
	}
	if got.ID != u.ID || got.Role != model.RoleUser {
		t.Fatalf("FindByUsername 返回不一致: %+v", got)
	}

	if _, err := repo.FindByUsername(ctx, "nobody"); err == nil {
		t.Fatal("查询不存在用户应返回错误")
	}
}

func TestUserRepo_ExistsByUsername_Scenarios(t *testing.T) {
	db := newTestDB(t, &model.UserDO{})
	repo := NewUserRepo(db)
	ctx := context.Background()

	if err := repo.Create(ctx, &model.UserDO{Username: "alice", Password: "hash"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	cases := []struct {
		name     string
		username string
		want     bool
	}{
		{name: "已存在", username: "alice", want: true},
		{name: "不存在", username: "bob", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ExistsByUsername(ctx, tc.username)
			if err != nil {
				t.Fatalf("ExistsByUsername 失败: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ExistsByUsername(%s) = %v, 期望 %v", tc.username, got, tc.want)
			}
		})
	}

	// 软删除后不再视为存在
	u, err := repo.FindByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByUsername 失败: %v", err)
	}
	if err := repo.SoftDelete(ctx, strconv.FormatInt(u.ID, 10)); err != nil {
		t.Fatalf("SoftDelete 失败: %v", err)
	}
	exists, err := repo.ExistsByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("ExistsByUsername 失败: %v", err)
	}
	if exists {
		t.Fatal("软删除后 ExistsByUsername 应为 false")
	}
}

func TestUserRepo_List_KeywordFilter(t *testing.T) {
	db := newTestDB(t, &model.UserDO{})
	repo := NewUserRepo(db)
	ctx := context.Background()

	for _, name := range []string{"alice", "alina", "bob"} {
		if err := repo.Create(ctx, &model.UserDO{Username: name, Password: "hash"}); err != nil {
			t.Fatalf("Create %s 失败: %v", name, err)
		}
	}

	cases := []struct {
		name      string
		keyword   string
		wantRows  int
		wantTotal int64
	}{
		{name: "无关键字返回全部", keyword: "", wantRows: 3, wantTotal: 3},
		{name: "关键字模糊匹配", keyword: "ali", wantRows: 2, wantTotal: 2},
		{name: "无匹配", keyword: "zzz", wantRows: 0, wantTotal: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := repo.List(ctx, model.PageQuery{Page: 1, Size: 10}, tc.keyword)
			if err != nil {
				t.Fatalf("List 失败: %v", err)
			}
			if len(rows) != tc.wantRows || total != tc.wantTotal {
				t.Fatalf("List(keyword=%q) 期望 rows=%d total=%d, 实际 rows=%d total=%d",
					tc.keyword, tc.wantRows, tc.wantTotal, len(rows), total)
			}
		})
	}

	// 按 id DESC 排序（对照现有 listUsersReal）
	rows, _, err := repo.List(ctx, model.PageQuery{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("List 应返回 3 条, 实际 %d", len(rows))
	}
	if rows[0].Username != "bob" {
		t.Fatalf("List 应按 id DESC, 首条应为 bob, 实际 %s", rows[0].Username)
	}
}
