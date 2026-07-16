// Package auth 认证 HTTP 层：参数绑定/校验 → AuthService → 渲染响应。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"goRAGENT/internal/middleware"
	authsvc "goRAGENT/internal/service/auth"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/response"
)

// Handler 认证接口处理器（依赖注入 AuthService，不直接访问 DB）。
type Handler struct {
	svc authsvc.AuthService
}

// NewHandler 创建认证 Handler。
func NewHandler(svc authsvc.AuthService) *Handler { return &Handler{svc: svc} }

// AuthRoutes 注册 /login /register 路由。
func (h *Handler) AuthRoutes(r *gin.RouterGroup) {
	r.POST("/login", h.Login)
	r.POST("/register", h.Register)
}

// credentialReq 登录/注册共用请求体。
type credentialReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 账号密码登录。
func (h *Handler) Login(c *gin.Context) {
	var req credentialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "请输入账号和密码"))
		return
	}

	result, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(statusOf(err), response.FromError(err))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

// Register 注册新用户。
func (h *Handler) Register(c *gin.Context) {
	var req credentialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "请输入账号和密码"))
		return
	}
	if len(req.Username) < 2 || len(req.Password) < 4 {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "账号至少2位, 密码至少4位"))
		return
	}

	result, err := h.svc.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(statusOf(err), response.FromError(err))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

// CurrentUser 查询当前登录用户信息（JWT 中间件之后调用）。
func (h *Handler) CurrentUser(c *gin.Context) {
	uid := middleware.GetUserID(c.Request.Context())
	if uid == "" {
		c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "未登录"))
		return
	}
	vo, err := h.svc.CurrentUser(c.Request.Context(), uid)
	if err != nil {
		// 与原实现一致：查询不到用户返回 404 + 业务错误码
		c.JSON(http.StatusNotFound, response.FromError(err))
		return
	}
	c.JSON(http.StatusOK, response.Success(vo))
}

// statusOf 错误码 → HTTP 状态码（保持重构前行为：401/409/400/500）。
func statusOf(err error) int {
	switch errs.CodeOf(err) {
	case errs.CodeNotLogin:
		return http.StatusUnauthorized // 账号不存在 / 密码错误
	case errs.CodeBusinessError:
		return http.StatusConflict // 账号已存在
	case errs.CodeParamError:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError // 注册入库 / 签发凭证失败
	}
}
