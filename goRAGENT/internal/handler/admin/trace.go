package admin

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

// ragTraceRunVO 匹配前端 RagTraceRun 类型
type ragTraceRunVO struct {
	TraceID        string `json:"traceId"`
	TraceName      string `json:"traceName,omitempty"`
	EntryMethod    string `json:"entryMethod,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	UserName       string `json:"userName,omitempty"`
	Username       string `json:"username,omitempty"`
	UserID         string `json:"userId,omitempty"`
	Status         string `json:"status,omitempty"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
	DurationMs     int64  `json:"durationMs,omitempty"`
	TtftMs         int64  `json:"ttftMs,omitempty"`
	Question       string `json:"question,omitempty"`
	StartTime      string `json:"startTime,omitempty"`
	EndTime        string `json:"endTime,omitempty"`
}

// ragTraceNodeVO 匹配前端 RagTraceNode 类型
type ragTraceNodeVO struct {
	TraceID      string `json:"traceId"`
	NodeID       string `json:"nodeId"`
	ParentNodeID string `json:"parentNodeId,omitempty"`
	Depth        int    `json:"depth,omitempty"`
	NodeType     string `json:"nodeType,omitempty"`
	NodeName     string `json:"nodeName,omitempty"`
	ClassName    string `json:"className,omitempty"`
	MethodName   string `json:"methodName,omitempty"`
	Status       string `json:"status,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	DurationMs   int64  `json:"durationMs,omitempty"`
	StartTime    string `json:"startTime,omitempty"`
	EndTime      string `json:"endTime,omitempty"`
}

// ragTraceDetailVO 匹配前端 RagTraceDetail 类型
type ragTraceDetailVO struct {
	Run   ragTraceRunVO   `json:"run"`
	Nodes []ragTraceNodeVO `json:"nodes"`
}

func runDOtoVO(r model.TraceRunDO) ragTraceRunVO {
	startTime := r.CreateTime.Format("2006-01-02 15:04:05")
	endTime := r.UpdateTime.Format("2006-01-02 15:04:05")
	if r.UpdateTime.Before(r.CreateTime) || r.UpdateTime.Equal(r.CreateTime) {
		endTime = ""
	}

	return ragTraceRunVO{
		TraceID: r.RunID, TraceName: "Chat", EntryMethod: "HTTP",
		ConversationID: r.ConversationID, TaskID: r.RunID,
		UserID: r.UserID, Username: r.UserID, UserName: r.UserID,
		Status: r.Status, ErrorMessage: r.ErrorMessage,
		Question: r.Question, StartTime: startTime, EndTime: endTime,
	}
}

func (h *Handler) listTraceRunsReal(c *gin.Context) {
	current, _ := strconv.Atoi(c.DefaultQuery("current", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if current < 1 {
		current = 1
	}
	if size < 1 {
		size = 10
	}
	traceID := c.Query("traceId")
	convID := c.Query("conversationId")

	var dos []model.TraceRunDO
	var total int64
	query := h.db.WithContext(c.Request.Context()).Model(&model.TraceRunDO{})
	if traceID != "" {
		query = query.Where("run_id = ?", traceID)
	}
	if convID != "" {
		query = query.Where("conversation_id = ?", convID)
	}
	query.Count(&total)
	if err := query.Order("create_time DESC").Offset((current - 1) * size).Limit(size).Find(&dos).Error; err != nil {
		zap.L().Error("查询 Trace 列表失败", zap.Error(err))
		c.JSON(http.StatusOK, response.Failure(response.CodeServerError, "查询失败"))
		return
	}

	vos := make([]ragTraceRunVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, runDOtoVO(d))
	}
	if vos == nil {
		vos = []ragTraceRunVO{}
	}
	pages := int(math.Ceil(float64(total) / float64(size)))

	c.JSON(http.StatusOK, response.Success(gin.H{
		"records": vos, "total": total, "size": size, "current": current, "pages": pages,
	}))
}

func (h *Handler) getTraceDetailReal(c *gin.Context) {
	runID := c.Param("traceId")
	if runID == "" {
		runID = c.Param("runId")
	}
	var run model.TraceRunDO
	if err := h.db.WithContext(c.Request.Context()).
		Where("run_id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusOK, response.Failure(response.CodeBusinessError, "Trace 不存在"))
		return
	}

	var nodeDOs []model.TraceNodeDO
	h.db.WithContext(c.Request.Context()).Where("run_id = ?", runID).Order("id ASC").Find(&nodeDOs)
	nodeVOs := make([]ragTraceNodeVO, 0, len(nodeDOs))
	for _, n := range nodeDOs {
		nodeVOs = append(nodeVOs, ragTraceNodeVO{
			TraceID: n.RunID, NodeID: strconv.FormatInt(n.ID, 10),
			ParentNodeID: n.ParentNodeID, NodeType: n.NodeType, NodeName: n.NodeName,
			Status: "DONE", DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}
	if nodeVOs == nil {
		nodeVOs = []ragTraceNodeVO{}
	}

	c.JSON(http.StatusOK, response.Success(ragTraceDetailVO{Run: runDOtoVO(run), Nodes: nodeVOs}))
}

func (h *Handler) getTraceNodesReal(c *gin.Context) {
	runID := c.Param("traceId")
	var nodeDOs []model.TraceNodeDO
	h.db.WithContext(c.Request.Context()).Where("run_id = ?", runID).Order("id ASC").Find(&nodeDOs)

	vos := make([]ragTraceNodeVO, 0, len(nodeDOs))
	for _, n := range nodeDOs {
		vos = append(vos, ragTraceNodeVO{
			TraceID: n.RunID, NodeID: strconv.FormatInt(n.ID, 10),
			ParentNodeID: n.ParentNodeID, NodeType: n.NodeType, NodeName: n.NodeName,
			Status: "DONE", DurationMs: n.DurationMs, ErrorMessage: n.ErrorMessage,
		})
	}
	if vos == nil {
		vos = []ragTraceNodeVO{}
	}
	c.JSON(http.StatusOK, response.Success(vos))
}

func (h *Handler) GetTraceNodes(c *gin.Context) { h.getTraceNodesReal(c) }
