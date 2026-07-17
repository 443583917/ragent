package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/model"
)

func (h *Handler) listUsersReal(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.User.List(c.Request.Context(), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) createUser(c *gin.Context) {
	var req model.UserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "用户名和密码不能为空")
		return
	}
	if err := h.svc.User.Create(c.Request.Context(), req); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) updateUser(c *gin.Context) {
	var req model.UserUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.User.Update(c.Request.Context(), c.Param("id"), req); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) deleteUser(c *gin.Context) {
	if err := h.svc.User.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) changePassword(c *gin.Context) {
	var req model.UserPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "密码不能为空")
		return
	}
	if err := h.svc.User.ChangePassword(c.Request.Context(), c.Param("id"), req.Password); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}
