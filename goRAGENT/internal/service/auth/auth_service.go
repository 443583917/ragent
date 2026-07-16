// Package auth 认证域服务：登录 / 注册 / 当前用户查询。
package auth

import (
	"context"
	"fmt"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/jwt"
)

// AuthService 认证业务接口。
type AuthService interface {
	Login(ctx context.Context, username, password string) (*model.LoginResult, error)
	Register(ctx context.Context, username, password string) (*model.LoginResult, error)
	CurrentUser(ctx context.Context, userID string) (*model.CurrentUserVO, error)
}

type authService struct {
	users  repository.UserRepository
	hasher PasswordHasher
}

// NewAuthService 创建认证服务。
func NewAuthService(users repository.UserRepository, hasher PasswordHasher) AuthService {
	return &authService{users: users, hasher: hasher}
}

// Login 账号密码登录，成功签发 JWT。
func (s *authService) Login(ctx context.Context, username, password string) (*model.LoginResult, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, errs.NotLogin("账号不存在")
	}
	if !s.hasher.Verify(password, user.Password) {
		return nil, errs.NotLogin("密码错误")
	}
	token, err := jwt.GenerateToken(fmt.Sprintf("%d", user.ID), user.Username, user.Role, user.Avatar)
	if err != nil {
		return nil, errs.WrapServer(err, "签发登录凭证失败")
	}
	return &model.LoginResult{Token: token, Username: user.Username, Role: user.Role}, nil
}

// Register 注册新用户（默认角色 user），成功后直接签发 JWT。
func (s *authService) Register(ctx context.Context, username, password string) (*model.LoginResult, error) {
	// 业务规则兜底（handler 已做参数校验，此处防止绕过 HTTP 层调用）
	if len(username) < 2 || len(password) < 4 {
		return nil, errs.Param("账号至少2位, 密码至少4位")
	}
	exists, err := s.users.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, errs.WrapServer(err, "注册失败")
	}
	if exists {
		return nil, errs.Business("账号已存在")
	}
	user := model.UserDO{Username: username, Password: s.hasher.Hash(password), Role: "user"}
	if err := s.users.Create(ctx, &user); err != nil {
		return nil, errs.WrapServer(err, "注册失败")
	}
	token, err := jwt.GenerateToken(fmt.Sprintf("%d", user.ID), user.Username, user.Role, "")
	if err != nil {
		return nil, errs.WrapServer(err, "签发登录凭证失败")
	}
	return &model.LoginResult{Token: token, Username: user.Username, Role: user.Role}, nil
}

// CurrentUser 按用户 ID 查询当前登录用户信息。
func (s *authService) CurrentUser(ctx context.Context, userID string) (*model.CurrentUserVO, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, errs.Business("用户不存在")
	}
	return &model.CurrentUserVO{
		UserID:   fmt.Sprintf("%d", user.ID),
		Username: user.Username,
		Role:     user.Role,
		Avatar:   user.Avatar,
	}, nil
}
