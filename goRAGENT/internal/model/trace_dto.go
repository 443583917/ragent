package model

// RagTraceRunVO 匹配前端 RagTraceRun 类型。
type RagTraceRunVO struct {
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

// RagTraceNodeVO 匹配前端 RagTraceNode 类型。
type RagTraceNodeVO struct {
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

// RagTraceDetailVO 匹配前端 RagTraceDetail 类型（包含 Run + Nodes）。
type RagTraceDetailVO struct {
	Run   RagTraceRunVO    `json:"run"`
	Nodes []RagTraceNodeVO `json:"nodes"`
}

// RunDOToVO TraceRunDO → RagTraceRunVO 转换。
// EndTime 逻辑：如果 UpdateTime < CreateTime 或相等则置空（保留现有 handler 行为）。
func RunDOToVO(r TraceRunDO) RagTraceRunVO {
	startTime := r.CreateTime.Format("2006-01-02 15:04:05")
	endTime := r.UpdateTime.Format("2006-01-02 15:04:05")
	if r.UpdateTime.Before(r.CreateTime) || r.UpdateTime.Equal(r.CreateTime) {
		endTime = ""
	}

	return RagTraceRunVO{
		TraceID: r.RunID, TraceName: "Chat", EntryMethod: "HTTP",
		ConversationID: r.ConversationID, TaskID: r.RunID,
		UserID: r.UserID, Username: r.UserID, UserName: r.UserID,
		Status: r.Status, ErrorMessage: r.ErrorMessage,
		Question: r.Question, StartTime: startTime, EndTime: endTime,
	}
}
