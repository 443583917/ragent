package auth

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/model"
	"goRAGENT/pkg/jwt"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/middleware"
	"gorm.io/gorm"
)

type Handler struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler { return &Handler{db: db, cfg: cfg} }

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/login", h.Login)
	r.POST("/register", h.Register)
}

func (h *Handler) AuthRoutes(r *gin.RouterGroup) {
	r.POST("/login", h.Login)
	r.POST("/register", h.Register)
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "请输入账号和密码"))
		return
	}

	var user model.UserDO
	err := h.db.Where("username = ? AND deleted = 0", req.Username).First(&user).Error
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "账号不存在"))
		return
	}
	if user.Password != model.MD5Hash(req.Password) {
		c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "密码错误"))
		return
	}

	token, _ := jwt.GenerateToken(fmt.Sprintf("%d", user.ID), user.Username, user.Role, user.Avatar)
	c.JSON(http.StatusOK, response.Success(gin.H{
		"token": token, "username": user.Username, "role": user.Role,
	}))
}

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "请输入账号和密码"))
		return
	}
	if len(req.Username) < 2 || len(req.Password) < 4 {
		c.JSON(http.StatusBadRequest, response.Failure(response.CodeParamError, "账号至少2位, 密码至少4位"))
		return
	}

	var exist int64
	h.db.Model(&model.UserDO{}).Where("username = ? AND deleted = 0", req.Username).Count(&exist)
	if exist > 0 {
		c.JSON(http.StatusConflict, response.Failure(response.CodeBusinessError, "账号已存在"))
		return
	}

	user := model.UserDO{Username: req.Username, Password: model.MD5Hash(req.Password), Role: "user"}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, response.Failure(response.CodeServerError, "注册失败"))
		return
	}

	token, _ := jwt.GenerateToken(fmt.Sprintf("%d", user.ID), user.Username, user.Role, "")
	c.JSON(http.StatusOK, response.Success(gin.H{
		"token": token, "username": user.Username, "role": user.Role,
	}))
}

func CurrentUser(db *gorm.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := middleware.GetUserID(c.Request.Context())
		if uid == "" {
			c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "未登录"))
			return
		}
		var user model.UserDO
		if err := db.Where("id = ?", uid).First(&user).Error; err != nil {
			c.JSON(http.StatusNotFound, response.Failure(response.CodeBusinessError, "用户不存在"))
			return
		}
		c.JSON(http.StatusOK, response.Success(gin.H{
			"userId": fmt.Sprintf("%d", user.ID), "username": user.Username,
			"role": user.Role, "avatar": user.Avatar,
		}))
	}
}
