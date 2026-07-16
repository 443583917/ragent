package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/pkg/snowflake"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

const mappingTimeLayout = "2006-01-02 15:04:05"

// mappingVO 关键词映射 VO
type mappingVO = model.MappingVO
type mappingCreateReq = model.MappingCreateReq
type mappingUpdateReq = model.MappingUpdateReq

// ========== Handler 方法 ==========

func (h *Handler) listMappings(c *gin.Context) {
	current, _ := strconv.Atoi(c.DefaultQuery("current", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if current < 1 {
		current = 1
	}
	if size < 1 || size > 200 {
		size = 10
	}
	keyword := strings.TrimSpace(c.Query("keyword"))

	q := h.db.WithContext(c.Request.Context()).Model(&model.TermMappingDO{}).Where("deleted = 0")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("source_term LIKE ? OR target_term LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		zap.L().Error("查询映射总数失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}
	var rows []model.TermMappingDO
	if err := q.Order("priority DESC, id DESC").
		Offset((current - 1) * size).Limit(size).
		Find(&rows).Error; err != nil {
		zap.L().Error("查询映射列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}
	vos := make([]mappingVO, len(rows))
	for i, r := range rows {
		vos[i] = model.MappingToVO(r)
	}
	c.JSON(http.StatusOK, response.Success(model.NewPageResult(vos, total, model.PageQuery{Page: current, Size: size})))
}

func (h *Handler) getMapping(c *gin.Context) {
	var row model.TermMappingDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).
		First(&row).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "映射不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(model.MappingToVO(row)))
}

func (h *Handler) createMapping(c *gin.Context) {
	var req mappingCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "sourceTerm/targetTerm 不能为空"))
		return
	}
	do := model.MappingCreateReqToDO(req, snowflake.NextID(), middleware.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).Create(&do).Error; err != nil {
		zap.L().Error("创建映射失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "创建失败"))
		return
	}
	h.clearMappingCache(c)
	c.JSON(http.StatusOK, response.Success(do.ID))
}

func (h *Handler) updateMapping(c *gin.Context) {
	var req mappingUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "参数错误"))
		return
	}
	updates := model.MappingUpdateReqToUpdates(req, middleware.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).Model(&model.TermMappingDO{}).
		Where("id = ? AND deleted = 0", c.Param("id")).
		Updates(updates).Error; err != nil {
		zap.L().Error("更新映射失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "更新失败"))
		return
	}
	h.clearMappingCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) deleteMapping(c *gin.Context) {
	if err := h.db.WithContext(c.Request.Context()).Model(&model.TermMappingDO{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{"deleted": 1, "update_by": middleware.GetUserID(c.Request.Context())}).Error; err != nil {
		zap.L().Error("删除映射失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "删除失败"))
		return
	}
	h.clearMappingCache(c)
	c.JSON(http.StatusOK, response.SuccessOK())
}

func (h *Handler) clearMappingCache(c *gin.Context) {
	if h.mappingCache != nil {
		h.mappingCache.ClearCache(c.Request.Context())
	}
}
