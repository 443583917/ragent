package admin

import (
	"context"
	"errors"
	"testing"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/internal/service/auth"
)

// mockUserRepo 手写 Mock UserRepository
type mockUserRepo struct {
	repository.UserRepository
	listFn             func(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error)
	existsByUsernameFn func(ctx context.Context, username string) (bool, error)
	createFn           func(ctx context.Context, u *model.UserDO) error
	updateFieldsFn     func(ctx context.Context, id string, updates map[string]any) error
	softDeleteFn       func(ctx context.Context, id string) error
}

func (m *mockUserRepo) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error) {
	return m.listFn(ctx, q, keyword)
}
func (m *mockUserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return m.existsByUsernameFn(ctx, username)
}
func (m *mockUserRepo) Create(ctx context.Context, u *model.UserDO) error {
	return m.createFn(ctx, u)
}
func (m *mockUserRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	return m.updateFieldsFn(ctx, id, updates)
}
func (m *mockUserRepo) SoftDelete(ctx context.Context, id string) error {
	return m.softDeleteFn(ctx, id)
}

func TestUserService_Create_DuplicateUsername(t *testing.T) {
	repo := &mockUserRepo{
		existsByUsernameFn: func(_ context.Context, username string) (bool, error) {
			if username == "existing" {
				return true, nil
			}
			return false, nil
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	err := svc.Create(context.Background(), model.UserCreateReq{
		Username: "existing",
		Password: "password123",
	})
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestUserService_Create_Success(t *testing.T) {
	repo := &mockUserRepo{
		existsByUsernameFn: func(_ context.Context, username string) (bool, error) {
			return false, nil
		},
		createFn: func(_ context.Context, u *model.UserDO) error {
			if u.Username != "newuser" {
				t.Errorf("expected username 'newuser', got %q", u.Username)
			}
			if u.Password == "" || u.Password == "password123" {
				t.Error("password should be hashed")
			}
			if u.Role != "user" {
				t.Errorf("expected default role 'user', got %q", u.Role)
			}
			return nil
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	err := svc.Create(context.Background(), model.UserCreateReq{
		Username: "newuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserService_Create_ShortUsername(t *testing.T) {
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(&mockUserRepo{}, hasher)

	err := svc.Create(context.Background(), model.UserCreateReq{
		Username: "a",
		Password: "password123",
	})
	if err == nil {
		t.Fatal("expected error for short username")
	}
}

func TestUserService_ChangePassword_Success(t *testing.T) {
	var updatedFields map[string]any
	repo := &mockUserRepo{
		updateFieldsFn: func(_ context.Context, id string, updates map[string]any) error {
			updatedFields = updates
			return nil
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	err := svc.ChangePassword(context.Background(), "user-1", "newPass123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedFields == nil {
		t.Fatal("expected updateFields to be called")
	}
	pwd, ok := updatedFields["password"].(string)
	if !ok || pwd == "" {
		t.Error("password field should be set and non-empty")
	}
	if pwd == "newPass123" {
		t.Error("password should be hashed, not plaintext")
	}
}

func TestUserService_ChangePassword_TooShort(t *testing.T) {
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(&mockUserRepo{}, hasher)

	err := svc.ChangePassword(context.Background(), "user-1", "abc")
	if err == nil {
		t.Fatal("expected error for too short password")
	}
}

func TestUserService_List(t *testing.T) {
	expectedUsers := []model.UserDO{
		{ID: 1, Username: "alice", Role: "admin"},
		{ID: 2, Username: "bob", Role: "user"},
	}
	repo := &mockUserRepo{
		listFn: func(_ context.Context, q model.PageQuery, _ string) ([]model.UserDO, int64, error) {
			return expectedUsers, 2, nil
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	vos, total, err := svc.List(context.Background(), model.PageQuery{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 || len(vos) != 2 {
		t.Errorf("expected 2 users, got total=%d len=%d", total, len(vos))
	}
	if vos[0].Username != "alice" || vos[1].Username != "bob" {
		t.Errorf("unexpected usernames: %+v", vos)
	}
}

func TestUserService_Delete(t *testing.T) {
	var deletedID string
	repo := &mockUserRepo{
		softDeleteFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	err := svc.Delete(context.Background(), "user-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != "user-42" {
		t.Errorf("expected to delete 'user-42', got %q", deletedID)
	}
}

func TestUserService_Delete_Error(t *testing.T) {
	repo := &mockUserRepo{
		softDeleteFn: func(_ context.Context, _ string) error {
			return errors.New("db error")
		},
	}
	hasher := auth.NewMD5PasswordHasher()
	svc := NewUserService(repo, hasher)

	err := svc.Delete(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when repo.SoftDelete fails")
	}
}
