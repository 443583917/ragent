package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

type ingestionTaskVO = model.IngestionTaskVO
type ingestionNodeVO = model.IngestionNodeVO

func (h *Handler) listIngestionTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []model.IngestionTaskDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&model.IngestionTaskDO{}).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Order("create_time DESC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询入库任务列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]ingestionTaskVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.TaskDOToVO(d))
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getIngestionTask(c *gin.Context) {
	var do model.IngestionTaskDO
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.db.WithContext(c.Request.Context()).First(&do, id).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "任务不存在"))
		return
	}
	c.JSON(http.StatusOK, response.Success(model.TaskDOToVO(do)))
}

func (h *Handler) getIngestionTaskNodes(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var do model.IngestionTaskDO
	if err := h.db.WithContext(c.Request.Context()).First(&do, id).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "任务不存在"))
		return
	}

	nodeNames := []string{"Fetcher", "Parser", "Chunker", "Indexer"}
	nodes := make([]ingestionNodeVO, 0, 4)
	for i, name := range nodeNames {
		status := "PENDING"
		switch do.Status {
		case model.TaskStatusDone:
			status = "DONE"
		case model.TaskStatusFailed:
			if do.CompletedChunks == 0 && i == 0 {
				status = "FAILED"
			} else if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else {
				status = "FAILED"
			}
		case model.TaskStatusRunning:
			if do.CompletedChunks > 0 && i < 3 {
				status = "DONE"
			} else if do.CompletedChunks > 0 {
				status = "RUNNING"
			} else if i == 0 {
				status = "RUNNING"
			}
		}
		nodes = append(nodes, ingestionNodeVO{Name: name, Status: status})
	}
	c.JSON(http.StatusOK, response.Success(nodes))
}
