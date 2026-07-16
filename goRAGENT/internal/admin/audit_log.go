package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type bizChangeLogVO struct {
	ID             int64  `json:"id"`
	EntityType     string `json:"entityType"`
	EntityID       string `json:"entityId"`
	Action         string `json:"action"`
	Operator       string `json:"operator"`
	BeforeSnapshot string `json:"beforeSnapshot,omitempty"`
	AfterSnapshot  string `json:"afterSnapshot,omitempty"`
	CreateTime     string `json:"createTime"`
}

func (h *Handler) listBizChangeLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	entityType := c.Query("entityType")

	var dos []rag.BizChangeLogDO
	var total int64
	query := h.db.WithContext(c.Request.Context()).Model(&rag.BizChangeLogDO{})
	if entityType != "" {
		query = query.Where("entity_type = ?", entityType)
	}
	query.Count(&total)
	if err := query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&dos).Error; err != nil {
		zap.L().Error("查询审计日志失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]bizChangeLogVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, bizChangeLogVO{
			ID: d.ID, EntityType: d.EntityType, EntityID: d.EntityID,
			Action: d.Action, Operator: d.Operator,
			BeforeSnapshot: d.BeforeSnapshot, AfterSnapshot: d.AfterSnapshot,
			CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getBizChangeLog(c *gin.Context) {
	var do rag.BizChangeLogDO
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.db.WithContext(c.Request.Context()).First(&do, id).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "日志不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(bizChangeLogVO{
		ID: do.ID, EntityType: do.EntityType, EntityID: do.EntityID,
		Action: do.Action, Operator: do.Operator,
		BeforeSnapshot: do.BeforeSnapshot, AfterSnapshot: do.AfterSnapshot,
		CreateTime: do.CreateTime.Format("2006-01-02 15:04:05"),
	}))
}

// WriteAudit 写入审计日志（供其他 handler 调用）
func (h *Handler) WriteAudit(entityType, entityID, action, operator, before, after string) {
	log := rag.BizChangeLogDO{
		EntityType: entityType, EntityID: entityID, Action: action,
		Operator: operator, BeforeSnapshot: before, AfterSnapshot: after,
	}
	if err := h.db.Create(&log).Error; err != nil {
		zap.L().Warn("审计日志写入失败", zap.Error(err))
	}
}
