package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/rag"
	"go.uber.org/zap"
)

type traceRunVO struct {
	ID             int64  `json:"id"`
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	Question       string `json:"question"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
	CreateTime     string `json:"createTime"`
}

type traceNodeVO struct {
	ID           int64  `json:"id"`
	NodeName     string `json:"nodeName"`
	NodeType     string `json:"nodeType"`
	DurationMs   int64  `json:"durationMs"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

func (h *Handler) listTraceRunsReal(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	var dos []rag.TraceRunDO
	var total int64
	h.db.WithContext(c.Request.Context()).Model(&rag.TraceRunDO{}).Count(&total)
	if err := h.db.WithContext(c.Request.Context()).
		Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&dos).Error; err != nil {
		zap.L().Error("查询 Trace 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]traceRunVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, traceRunVO{
			ID: d.ID, RunID: d.RunID, ConversationID: d.ConversationID,
			UserID: d.UserID, Question: d.Question, Status: d.Status,
			ErrorMessage: d.ErrorMessage, CreateTime: d.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, response.Success(gin.H{"total": total, "rows": vos}))
}

func (h *Handler) getTraceDetailReal(c *gin.Context) {
	runID := c.Param("runId")
	var run rag.TraceRunDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("run_id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Trace 不存在"))
		return
	}

	var nodes []rag.TraceNodeDO
	h.db.WithContext(c.Request.Context()).
		Where("run_id = ?", runID).Order("id ASC").Find(&nodes)

	nodeVOs := make([]traceNodeVO, 0, len(nodes))
	for _, n := range nodes {
		nodeVOs = append(nodeVOs, traceNodeVO{
			ID: n.ID, NodeName: n.NodeName, NodeType: n.NodeType,
			DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}

	c.JSON(http.StatusOK, response.Success(gin.H{
		"run": traceRunVO{
			ID: run.ID, RunID: run.RunID, ConversationID: run.ConversationID,
			UserID: run.UserID, Question: run.Question, Status: run.Status,
			ErrorMessage: run.ErrorMessage, CreateTime: run.CreateTime.Format("2006-01-02 15:04:05"),
		},
		"nodes": nodeVOs,
	}))
}

func (h *Handler) getTraceNodesReal(c *gin.Context) {
	runID := c.Param("traceId")
	var nodes []rag.TraceNodeDO
	h.db.WithContext(c.Request.Context()).
		Where("run_id = ?", runID).Order("id ASC").Find(&nodes)

	vos := make([]traceNodeVO, 0, len(nodes))
	for _, n := range nodes {
		vos = append(vos, traceNodeVO{
			ID: n.ID, NodeName: n.NodeName, NodeType: n.NodeType,
			DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) GetTraceNodes(c *gin.Context) { h.getTraceNodesReal(c) }

// TraceWriter 供 Pipeline 写入 trace 记录
type TraceWriter struct {
	db *rag.TraceRunDO // not used directly, we go through Handler
}

func (h *Handler) WriteTraceRun(run *rag.TraceRunDO) error {
	return h.db.Create(run).Error
}

func (h *Handler) WriteTraceNodes(nodes []rag.TraceNodeDO) error {
	if len(nodes) == 0 {
		return nil
	}
	return h.db.Create(&nodes).Error
}
