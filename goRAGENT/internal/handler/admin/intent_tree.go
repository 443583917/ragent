package admin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/pkg/snowflake"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

// CacheClearer 意图树缓存清除抽象（*model.TreeLoader 满足）
type CacheClearer interface {
	ClearCache(ctx context.Context)
}

// IntentNodeTreeVO 意图树节点 VO
type IntentNodeTreeVO = model.IntentNodeTreeVO
type intentNodeCreateReq = model.IntentNodeCreateReq
type intentNodeUpdateReq = model.IntentNodeUpdateReq
type intentNodeBatchReq = model.IntentNodeBatchReq

// ========== Handler 方法 ==========

func (h *Handler) clearIntentCache(c *gin.Context) {
	if h.intentCache != nil {
		h.intentCache.ClearCache(c.Request.Context())
	}
}

func (h *Handler) intentTrees(c *gin.Context) {
	var dos []model.IntentNodeDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("deleted = 0").Find(&dos).Error; err != nil {
		zap.L().Error("查询意图树失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询意图树失败"))
		return
	}
	vos := model.BuildIntentTreeVOs(dos)
	if vos == nil {
		vos = []*IntentNodeTreeVO{}
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) createIntentNode(c *gin.Context) {
	var req intentNodeCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "intentCode/name 不能为空"))
		return
	}
	do := model.IntentCreateReqToDO(req, snowflake.NextID(), middleware.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败（intentCode 可能重复）"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.Success(do.ID))
}

func (h *Handler) updateIntentNode(c *gin.Context) {
	var req intentNodeUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := model.IntentUpdateReqToUpdates(req, middleware.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.IntentNodeDO{}).
		Where("id = ? AND deleted = 0", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteIntentNode(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.IntentNodeDO{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{"deleted": 1, "update_by": middleware.GetUserID(c.Request.Context())}).Error; err != nil {
		zap.L().Error("删除意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) batchUpdateIntent(c *gin.Context, updates map[string]any) {
	var req intentNodeBatchReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "ids 不能为空"))
		return
	}
	updates["update_by"] = middleware.GetUserID(c.Request.Context())
	if err := h.db.WithContext(c.Request.Context()).
		Model(&model.IntentNodeDO{}).
		Where("id IN ?", req.Ids).
		Updates(updates).Error; err != nil {
		zap.L().Error("批量更新意图节点失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "批量操作失败"))
		return
	}
	h.clearIntentCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}
