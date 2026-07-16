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
