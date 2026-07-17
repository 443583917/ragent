package session

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/service/rag"
	"goRAGENT/pkg/response"
)

// Handler 会话/消息/反馈 HTTP handler（无业务逻辑，仅 HTTP 编排）。
type Handler struct {
	svc rag.SessionService
}

// NewHandler 创建会话 HTTP handler。
func NewHandler(svc rag.SessionService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 注册前端实际使用的 /conversations 契约路由（调用方需已挂 JWT 中间件）。
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/conversations", h.ListSessions)
	r.PUT("/conversations/:conversationId", h.RenameSession)
	r.DELETE("/conversations/:conversationId", h.DeleteSession)
	r.GET("/conversations/:conversationId/messages", h.ListMessages)
	r.POST("/conversations/messages/:messageId/feedback", h.SubmitFeedback)
	r.DELETE("/conversations/messages/:messageId/feedback", h.CancelFeedback)
}

func (h *Handler) uid(c *gin.Context) (string, bool) {
	uid := middleware.GetUserID(c.Request.Context())
	if uid == "" {
		c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "未登录"))
		return "", false
	}
	return uid, true
}

// ListSessions GET /conversations
func (h *Handler) ListSessions(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	vos, err := h.svc.ListSessions(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询会话失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

// RenameSession PUT /conversations/:conversationId
func (h *Handler) RenameSession(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "title 不能为空"))
		return
	}
	if err := h.svc.RenameSession(c.Request.Context(), c.Param("conversationId"), uid, req.Title); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "重命名失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

// DeleteSession DELETE /conversations/:conversationId
func (h *Handler) DeleteSession(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteSession(c.Request.Context(), c.Param("conversationId"), uid); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

// ListMessages GET /conversations/:conversationId/messages
func (h *Handler) ListMessages(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	vos, err := h.svc.ListMessages(c.Request.Context(), c.Param("conversationId"), uid)
	if err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询消息失败"))
		return
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

// SubmitFeedback POST /conversations/messages/:messageId/feedback
func (h *Handler) SubmitFeedback(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	var req struct {
		Vote *int `json:"vote" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "vote 不能为空"))
		return
	}
	if err := h.svc.SubmitFeedback(c.Request.Context(), c.Param("messageId"), uid, req.Vote); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "反馈失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

// CancelFeedback DELETE /conversations/messages/:messageId/feedback
func (h *Handler) CancelFeedback(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	if err := h.svc.CancelFeedback(c.Request.Context(), c.Param("messageId"), uid); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "反馈失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
