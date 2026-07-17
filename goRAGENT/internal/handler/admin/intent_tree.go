package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
)

func (h *Handler) intentTrees(c *gin.Context) {
	vos, err := h.svc.Intent.GetTrees(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}
	if vos == nil {
		vos = []*model.IntentNodeTreeVO{}
	}
	httpx.OK(c, vos)
}

func (h *Handler) createIntentNode(c *gin.Context) {
	var req model.IntentNodeCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "intentCode/name 不能为空")
		return
	}
	id, err := h.svc.Intent.Create(c.Request.Context(), req, middleware.GetUserID(c.Request.Context()))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, id)
}

func (h *Handler) updateIntentNode(c *gin.Context) {
	var req model.IntentNodeUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Intent.Update(c.Request.Context(), c.Param("id"), req, middleware.GetUserID(c.Request.Context())); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) deleteIntentNode(c *gin.Context) {
	if err := h.svc.Intent.Delete(c.Request.Context(), c.Param("id"), middleware.GetUserID(c.Request.Context())); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) batchUpdateIntent(c *gin.Context, updates map[string]any) {
	var req model.IntentNodeBatchReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		httpx.BadRequest(c, "ids 不能为空")
		return
	}
	if err := h.svc.Intent.BatchUpdate(c.Request.Context(), req.Ids, updates, middleware.GetUserID(c.Request.Context())); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}
