package model

// LoginResult 登录/注册成功返回（JSON 字段与前端契约一致：token/username/role）
type LoginResult struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CurrentUserVO 当前登录用户信息（userId 为字符串形式的用户 ID）
type CurrentUserVO struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar"`
}

// ========== 用户管理 DTO ==========

// UserVO 用户管理列表 VO。
type UserVO struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar,omitempty"`
}

// UserCreateReq 创建用户请求体。
type UserCreateReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

// UserUpdateReq 更新用户请求体。
type UserUpdateReq struct {
	Role   *string `json:"role"`
	Avatar *string `json:"avatar"`
}

// UserPasswordReq 修改密码请求体。
type UserPasswordReq struct {
	Password string `json:"password" binding:"required"`
}
