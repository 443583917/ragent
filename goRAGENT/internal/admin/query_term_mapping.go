package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/response"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/snowflake"
	"github.com/nageoffer/ragent/goRAGENT/internal/framework/userctx"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/rewrite"
	"go.uber.org/zap"
)

const mappingTimeLayout = "2006-01-02 15:04:05"

// mappingVO 关键词映射 VO（和 Java QueryTermMappingVO / 前端 QueryTermMapping 一致）
type mappingVO struct {
	ID         string `json:"id"`
	SourceTerm string `json:"sourceTerm"`
	TargetTerm string `json:"targetTerm"`
	MatchType  int    `json:"matchType"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark,omitempty"`
	CreateTime string `json:"createTime,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
}

// pageResult MyBatis-Plus IPage 风格分页（和前端 PageResult 一致）
type pageResult struct {
	Records []mappingVO `json:"records"`
	Total   int64       `json:"total"`
	Size    int         `json:"size"`
	Current int         `json:"current"`
	Pages   int64       `json:"pages"`
}

type mappingCreateReq struct {
	SourceTerm string  `json:"sourceTerm" binding:"required"`
	TargetTerm string  `json:"targetTerm" binding:"required"`
	MatchType  *int    `json:"matchType"`
	Priority   *int    `json:"priority"`
	Enabled    *bool   `json:"enabled"`
	Remark     *string `json:"remark"`
}

type mappingUpdateReq struct {
	SourceTerm *string `json:"sourceTerm"`
	TargetTerm *string `json:"targetTerm"`
	MatchType  *int    `json:"matchType"`
	Priority   *int    `json:"priority"`
	Enabled    *bool   `json:"enabled"`
	Remark     *string `json:"remark"`
}

// ========== 纯转换函数 ==========

func mappingToVO(d rewrite.TermMappingDO) mappingVO {
	vo := mappingVO{
		ID: d.ID, SourceTerm: d.SourceTerm, TargetTerm: d.TargetTerm,
		MatchType: d.MatchType, Priority: d.Priority,
		Enabled: d.Enabled == 1, Remark: d.Remark,
	}
	if !d.CreateTime.IsZero() {
		vo.CreateTime = d.CreateTime.Format(mappingTimeLayout)
	}
	if !d.UpdateTime.IsZero() {
		vo.UpdateTime = d.UpdateTime.Format(mappingTimeLayout)
	}
	return vo
}

func mappingCreateReqToDO(req mappingCreateReq, id, operator string) rewrite.TermMappingDO {
	do := rewrite.TermMappingDO{
		ID: id, SourceTerm: req.SourceTerm, TargetTerm: req.TargetTerm,
		MatchType: rewrite.MatchTypeExact, Priority: 0, Enabled: 1,
		CreateBy: operator, UpdateBy: operator,
	}
	if req.MatchType != nil {
		do.MatchType = *req.MatchType
	}
	if req.Priority != nil {
		do.Priority = *req.Priority
	}
	if req.Enabled != nil && !*req.Enabled {
		do.Enabled = 0
	}
	if req.Remark != nil {
		do.Remark = *req.Remark
	}
	return do
}

func mappingUpdateReqToUpdates(req mappingUpdateReq, operator string) map[string]any {
	updates := map[string]any{"update_by": operator}
	if req.SourceTerm != nil {
		updates["source_term"] = *req.SourceTerm
	}
	if req.TargetTerm != nil {
		updates["target_term"] = *req.TargetTerm
	}
	if req.MatchType != nil {
		updates["match_type"] = *req.MatchType
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		v := 0
		if *req.Enabled {
			v = 1
		}
		updates["enabled"] = v
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	return updates
}

func buildPageResult(records []mappingVO, total int64, current, size int) pageResult {
	if records == nil {
		records = []mappingVO{}
	}
	pages := (total + int64(size) - 1) / int64(size)
	return pageResult{Records: records, Total: total, Size: size, Current: current, Pages: pages}
}

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

	q := h.db.WithContext(c.Request.Context()).Model(&rewrite.TermMappingDO{}).Where("deleted = 0")
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
	var rows []rewrite.TermMappingDO
	if err := q.Order("priority DESC, id DESC").
		Offset((current - 1) * size).Limit(size).
		Find(&rows).Error; err != nil {
		zap.L().Error("查询映射列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}
	vos := make([]mappingVO, len(rows))
	for i, r := range rows {
		vos[i] = mappingToVO(r)
	}
	c.JSON(http.StatusOK, response.Success(buildPageResult(vos, total, current, size)))
}

func (h *Handler) getMapping(c *gin.Context) {
	var row rewrite.TermMappingDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted = 0", c.Param("id")).
		First(&row).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "映射不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(mappingToVO(row)))
}

func (h *Handler) createMapping(c *gin.Context) {
	var req mappingCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeParamError, "sourceTerm/targetTerm 不能为空"))
		return
	}
	do := mappingCreateReqToDO(req, snowflake.NextID(), userctx.GetUserID(c.Request.Context()))
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
	updates := mappingUpdateReqToUpdates(req, userctx.GetUserID(c.Request.Context()))
	if err := h.db.WithContext(c.Request.Context()).Model(&rewrite.TermMappingDO{}).
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
	if err := h.db.WithContext(c.Request.Context()).Model(&rewrite.TermMappingDO{}).
		Where("id = ?", c.Param("id")).
		Updates(map[string]any{"deleted": 1, "update_by": userctx.GetUserID(c.Request.Context())}).Error; err != nil {
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
