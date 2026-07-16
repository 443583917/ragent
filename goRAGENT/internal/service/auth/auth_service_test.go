package auth

import (
	"context"
	"errors"
	"testing"

	"goRAGENT/internal/model"
	"goRAGENT/pkg/errs"
)

// 期望错误码别名，避免测试里手写魔法字符串
const (
	errCodeNotLogin = errs.CodeNotLogin
	errCodeParam    = errs.CodeParamError
	errCodeBusiness = errs.CodeBusinessError
	errCodeServer   = errs.CodeServerError
)

// requireErrCode 断言 err 非空且错误码匹配。
func requireErrCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际 err 为 nil", wantCode)
	}
	if got := errs.CodeOf(err); got != wantCode {
		t.Fatalf("错误码 = %s，want %s（err=%v）", got, wantCode, err)
	}
}

// mockUserRepo 手写 mock，实现 repository.UserRepository。
type mockUserRepo struct {
	findByIDFn         func(ctx context.Context, id string) (*model.UserDO, error)
	findByUsernameFn   func(ctx context.Context, username string) (*model.UserDO, error)
	existsByUsernameFn func(ctx context.Context, username string) (bool, error)
	createFn           func(ctx context.Context, u *model.UserDO) error
}

func (m *mockUserRepo) FindByID(ctx context.Context, id string) (*model.UserDO, error) {
	return m.findByIDFn(ctx, id)
}

func (m *mockUserRepo) FindByUsername(ctx context.Context, username string) (*model.UserDO, error) {
	return m.findByUsernameFn(ctx, username)
}

func (m *mockUserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return m.existsByUsernameFn(ctx, username)
}

func (m *mockUserRepo) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (m *mockUserRepo) Create(ctx context.Context, u *model.UserDO) error {
	return m.createFn(ctx, u)
}

func (m *mockUserRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	return errors.New("not implemented")
}

func (m *mockUserRepo) SoftDelete(ctx context.Context, id string) error {
	return errors.New("not implemented")
}

func TestMD5PasswordHasher_Hash_兼容存量MD5(t *testing.T) {
	h := NewMD5PasswordHasher()
	// md5("123456") 的十六进制小写，兼容存量数据
	want := "e10adc3949ba59abbe56e057f20f883e"
	if got := h.Hash("123456"); got != want {
		t.Fatalf("Hash(123456) = %s, want %s", got, want)
	}
	if !h.Verify("123456", want) {
		t.Fatal("Verify 正确密码应返回 true")
	}
	if h.Verify("654321", want) {
		t.Fatal("Verify 错误密码应返回 false")
	}
}

func TestAuthService_Login(t *testing.T) {
	hasher := NewMD5PasswordHasher()
	stored := &model.UserDO{ID: 42, Username: "alice", Password: hasher.Hash("pass1234"), Role: "admin", Avatar: "a.png"}

	tests := []struct {
		name     string
		repo     *mockUserRepo
		username string
		password string
		wantCode string // 期望错误码；空串表示期望成功
		wantRole string
	}{
		{
			name: "登录成功",
			repo: &mockUserRepo{findByUsernameFn: func(ctx context.Context, username string) (*model.UserDO, error) {
				if username != "alice" {
					t.Fatalf("unexpected username %s", username)
				}
				return stored, nil
			}},
			username: "alice", password: "pass1234",
			wantCode: "", wantRole: "admin",
		},
		{
			name: "账号不存在",
			repo: &mockUserRepo{findByUsernameFn: func(ctx context.Context, username string) (*model.UserDO, error) {
				return nil, errors.New("record not found")
			}},
			username: "bob", password: "pass1234",
			wantCode: errCodeNotLogin,
		},
		{
			name: "密码错误",
			repo: &mockUserRepo{findByUsernameFn: func(ctx context.Context, username string) (*model.UserDO, error) {
				return stored, nil
			}},
			username: "alice", password: "wrong",
			wantCode: errCodeNotLogin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAuthService(tt.repo, hasher)
			got, err := svc.Login(context.Background(), tt.username, tt.password)
			if tt.wantCode != "" {
				requireErrCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("Login() error = %v, want nil", err)
			}
			if got.Token == "" {
				t.Fatal("Login() token 不应为空")
			}
			if got.Username != tt.username || got.Role != tt.wantRole {
				t.Fatalf("Login() = %+v, want username=%s role=%s", got, tt.username, tt.wantRole)
			}
		})
	}
}

func TestAuthService_Register(t *testing.T) {
	hasher := NewMD5PasswordHasher()

	tests := []struct {
		name     string
		repo     *mockUserRepo
		username string
		password string
		wantCode string
	}{
		{
			name: "注册成功",
			repo: &mockUserRepo{
				existsByUsernameFn: func(ctx context.Context, username string) (bool, error) { return false, nil },
				createFn: func(ctx context.Context, u *model.UserDO) error {
					if u.Password != hasher.Hash("pass1234") {
						t.Fatalf("入库密码应为 MD5 哈希，got %s", u.Password)
					}
					if u.Role != "user" {
						t.Fatalf("默认角色应为 user，got %s", u.Role)
					}
					u.ID = 100
					return nil
				},
			},
			username: "carol", password: "pass1234",
			wantCode: "",
		},
		{
			name: "注册冲突_账号已存在",
			repo: &mockUserRepo{
				existsByUsernameFn: func(ctx context.Context, username string) (bool, error) { return true, nil },
			},
			username: "alice", password: "pass1234",
			wantCode: errCodeBusiness,
		},
		{
			name: "注册入库失败",
			repo: &mockUserRepo{
				existsByUsernameFn: func(ctx context.Context, username string) (bool, error) { return false, nil },
				createFn:           func(ctx context.Context, u *model.UserDO) error { return errors.New("db down") },
			},
			username: "dave", password: "pass1234",
			wantCode: errCodeServer,
		},
		{
			name:     "业务兜底_用户名过短",
			repo:     &mockUserRepo{},
			username: "x", password: "pass1234",
			wantCode: errCodeParam,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAuthService(tt.repo, hasher)
			got, err := svc.Register(context.Background(), tt.username, tt.password)
			if tt.wantCode != "" {
				requireErrCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("Register() error = %v, want nil", err)
			}
			if got.Token == "" || got.Username != tt.username || got.Role != "user" {
				t.Fatalf("Register() = %+v", got)
			}
		})
	}
}

func TestAuthService_CurrentUser(t *testing.T) {
	hasher := NewMD5PasswordHasher()

	t.Run("查询成功", func(t *testing.T) {
		repo := &mockUserRepo{findByIDFn: func(ctx context.Context, id string) (*model.UserDO, error) {
			return &model.UserDO{ID: 42, Username: "alice", Role: "admin", Avatar: "a.png"}, nil
		}}
		svc := NewAuthService(repo, hasher)
		got, err := svc.CurrentUser(context.Background(), "42")
		if err != nil {
			t.Fatalf("CurrentUser() error = %v", err)
		}
		if got.UserID != "42" || got.Username != "alice" || got.Role != "admin" || got.Avatar != "a.png" {
			t.Fatalf("CurrentUser() = %+v", got)
		}
	})

	t.Run("用户不存在", func(t *testing.T) {
		repo := &mockUserRepo{findByIDFn: func(ctx context.Context, id string) (*model.UserDO, error) {
			return nil, errors.New("record not found")
		}}
		svc := NewAuthService(repo, hasher)
		_, err := svc.CurrentUser(context.Background(), "999")
		requireErrCode(t, err, errCodeBusiness)
	})
}
