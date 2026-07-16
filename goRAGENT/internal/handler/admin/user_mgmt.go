package admin

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"go.uber.org/zap"
)

type userVO struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar,omitempty"`
}

type userCreateReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

type userUpdateReq struct {
	Role   *string `json:"role"`
	Avatar *string `json:"avatar"`
}

type userPasswordReq struct {
	Password string `json:"password" binding:"required"`
}

func (h *Handler) listUsersReal(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	type userRow struct {
		ID       int64  `gorm:"column:id"`
		Username string `gorm:"column:username"`
		Role     string `gorm:"column:role"`
		Avatar   string `gorm:"column:avatar"`
	}
	var rows []userRow
	var total int64
	h.db.WithContext(c.Request.Context()).Table("t_user").Where("deleted = 0").Count(&total)
	if err := h.db.WithContext(c.Request.Context()).Table("t_user").
		Where("deleted = 0").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&rows).Error; err != nil {
		zap.L().Error("查询用户列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]userVO, 0, len(rows))
	for _, r := range rows {
		vos = append(vos, userVO{ID: r.ID, Username: r.Username, Role: r.Role, Avatar: r.Avatar})
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) createUser(c *gin.Context) {
	var req userCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "用户名和密码不能为空"))
		return
	}
	if len(req.Username) < 2 || len(req.Password) < 4 {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "账号至少2位，密码至少4位"))
		return
	}

	var exist int64
	h.db.WithContext(c.Request.Context()).Table("t_user").Where("username = ? AND deleted = 0", req.Username).Count(&exist)
	if exist > 0 {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "账号已存在"))
		return
	}

	role := req.Role
	if role == "" {
		role = "user"
	}
	// MD5 hashing (same as user.go pattern)
	pwdHash := md5Hash(req.Password)

	if err := h.db.WithContext(c.Request.Context()).Exec(
		"INSERT INTO t_user (username, password, role) VALUES (?, ?, ?)", req.Username, pwdHash, role,
	).Error; err != nil {
		zap.L().Error("创建用户失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) updateUser(c *gin.Context) {
	var req userUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := map[string]any{}
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, response.SuccessOK())
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Table("t_user").
		Where("id = ? AND deleted = 0", c.Param("id")).Updates(updates).Error; err != nil {
		zap.L().Error("更新用户失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteUser(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).Table("t_user").
		Where("id = ?", c.Param("id")).Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除用户失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) changePassword(c *gin.Context) {
	var req userPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "密码不能为空"))
		return
	}
	if len(req.Password) < 4 {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "密码至少4位"))
		return
	}
	pwdHash := md5Hash(req.Password)
	if err := h.db.WithContext(c.Request.Context()).Table("t_user").
		Where("id = ? AND deleted = 0", c.Param("id")).Update("password", pwdHash).Error; err != nil {
		zap.L().Error("修改密码失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "修改失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func md5Hash(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }
