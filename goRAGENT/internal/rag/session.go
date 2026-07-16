package rag

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/framework/userctx"
	"goRAGENT/internal/rag/memory"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const timeLayout = "2006-01-02 15:04:05"

// ConversationVO 会话 VO（和前端 sessionService.ConversationVO 一致）
type ConversationVO struct {
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	LastTime       string `json:"lastTime,omitempty"`
}

// ConversationMessageVO 消息 VO（和前端 ConversationMessageVO 一致）
type ConversationMessageVO struct {
	ID               int64  `json:"id"`
	ConversationID   string `json:"conversationId"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	ThinkingContent  string `json:"thinkingContent,omitempty"`
	ThinkingDuration *int   `json:"thinkingDuration"`
	Vote             *int   `json:"vote"`
	CreateTime       string `json:"createTime,omitempty"`
}

func convToVO(d memory.ConversationDO) ConversationVO {
	vo := ConversationVO{ConversationID: d.ConversationID, Title: d.Title}
	if !d.LastTime.IsZero() {
		vo.LastTime = d.LastTime.Format(timeLayout)
	}
	return vo
}

func msgToVO(d memory.ConversationMessageDO) ConversationMessageVO {
	vo := ConversationMessageVO{
		ID: d.ID, ConversationID: d.ConversationID, Role: d.Role, Content: d.Content,
		ThinkingContent: d.ThinkingContent, ThinkingDuration: d.ThinkingDuration, Vote: d.Vote,
	}
	if !d.CreateTime.IsZero() {
		vo.CreateTime = d.CreateTime.Format(timeLayout)
	}
	return vo
}

// SessionHandler 会话/消息/反馈 API（和 Java ConversationController + MessageFeedbackController 对应）
type SessionHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewSessionHandler(db *gorm.DB, cfg *config.Config) *SessionHandler {
	return &SessionHandler{db: db, cfg: cfg}
}

// RegisterRoutes 注册前端实际使用的 /conversations 契约路由（调用方需已挂 JWT 中间件）
func (h *SessionHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/conversations", h.ListSessions)
	r.PUT("/conversations/:conversationId", h.RenameSession)
	r.DELETE("/conversations/:conversationId", h.DeleteSession)
	r.GET("/conversations/:conversationId/messages", h.ListMessages)
	r.POST("/conversations/messages/:messageId/feedback", h.SubmitFeedback)
	r.DELETE("/conversations/messages/:messageId/feedback", h.CancelFeedback)
}

func (h *SessionHandler) uid(c *gin.Context) (string, bool) {
	uid := userctx.GetUserID(c.Request.Context())
	if uid == "" {
		c.JSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "未登录"))
		return "", false
	}
	return uid, true
}

func (h *SessionHandler) ListSessions(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	var rows []memory.ConversationDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND deleted = 0", uid).
		Order("last_time DESC").Limit(200).
		Find(&rows).Error; err != nil {
		zap.L().Error("查询会话列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询会话失败"))
		return
	}
	vos := make([]ConversationVO, len(rows))
	for i, r := range rows {
		vos[i] = convToVO(r)
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *SessionHandler) RenameSession(c *gin.Context) {
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
	if err := h.db.WithContext(c.Request.Context()).Model(&memory.ConversationDO{}).
		Where("conversation_id = ? AND user_id = ? AND deleted = 0", c.Param("conversationId"), uid).
		Update("title", req.Title).Error; err != nil {
		zap.L().Error("重命名会话失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "重命名失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *SessionHandler) DeleteSession(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Model(&memory.ConversationDO{}).
		Where("conversation_id = ? AND user_id = ?", c.Param("conversationId"), uid).
		Update("deleted", 1).Error; err != nil {
		zap.L().Error("删除会话失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *SessionHandler) ListMessages(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	var rows []memory.ConversationMessageDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("conversation_id = ? AND user_id = ? AND deleted = 0", c.Param("conversationId"), uid).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		zap.L().Error("查询会话消息失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询消息失败"))
		return
	}
	vos := make([]ConversationMessageVO, len(rows))
	for i, r := range rows {
		vos[i] = msgToVO(r)
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *SessionHandler) SubmitFeedback(c *gin.Context) {
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
	h.updateVote(c, uid, req.Vote)
}

func (h *SessionHandler) CancelFeedback(c *gin.Context) {
	uid, ok := h.uid(c)
	if !ok {
		return
	}
	h.updateVote(c, uid, nil)
}

func (h *SessionHandler) updateVote(c *gin.Context, uid string, vote *int) {
	if err := h.db.WithContext(c.Request.Context()).Model(&memory.ConversationMessageDO{}).
		Where("id = ? AND user_id = ?", c.Param("messageId"), uid).
		Update("vote", vote).Error; err != nil {
		zap.L().Error("更新反馈失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "反馈失败"))
		return
	}
	c.JSON(http.StatusOK, response.SuccessOK())
}
