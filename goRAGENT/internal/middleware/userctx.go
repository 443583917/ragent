package middleware

import (
	"context"
	"fmt"
)

// LoginUser 登录用户信息
type LoginUser struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`   // "admin" | "user"
	Avatar   string `json:"avatar"`
}

// contextKey 用于 context.Context 的类型安全 key
type contextKey struct{}

var userKey = contextKey{}

// Set 将用户信息注入 context（请求入口 middleware 调用）
func Set(ctx context.Context, user *LoginUser) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// Get 从 context 获取用户信息（业务代码调用）
func Get(ctx context.Context) (*LoginUser, error) {
	user, ok := ctx.Value(userKey).(*LoginUser)
	if !ok || user == nil {
		return nil, fmt.Errorf("未获取到当前登录用户")
	}
	return user, nil
}

// GetUserID 获取当前用户 ID（返回空字符串表示未登录，不抛错）
func GetUserID(ctx context.Context) string {
	user, err := Get(ctx)
	if err != nil {
		return ""
	}
	return user.UserID
}

// HasUser 检查 context 中是否有用户信息
func HasUser(ctx context.Context) bool {
	_, err := Get(ctx)
	return err == nil
}
