package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/model"
	"go.uber.org/zap"
)

// ragTraceRunVO 匹配前端 RagTraceRun 类型
type ragTraceRunVO = model.RagTraceRunVO

// ragTraceNodeVO 匹配前端 RagTraceNode 类型
type ragTraceNodeVO = model.RagTraceNodeVO

// ragTraceDetailVO 匹配前端 RagTraceDetail 类型
type ragTraceDetailVO = model.RagTraceDetailVO

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
		vos = append(vos, model.RunDOToVO(d))
	}
	if vos == nil {
		vos = []ragTraceRunVO{}
	}

	c.JSON(http.StatusOK, response.Success(model.NewPageResult(vos, total, model.PageQuery{Page: current, Size: size})))
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

	c.JSON(http.StatusOK, response.Success(ragTraceDetailVO{Run: model.RunDOToVO(run), Nodes: nodeVOs}))
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
