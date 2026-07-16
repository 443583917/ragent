package sse

// SSEEventType SSE 事件类型枚举（和 Java 版 SSEEventType 完全一致）
type SSEEventType string

const (
	EventMeta    SSEEventType = "meta"
	EventMessage SSEEventType = "message"
	EventFinish  SSEEventType = "finish"
	EventDone    SSEEventType = "done"
	EventCancel  SSEEventType = "cancel"
	EventReject  SSEEventType = "reject"
)

func (e SSEEventType) Value() string { return string(e) }

// ========== 事件载荷（和 Java 版字段名/结构完全一致）==========

// MetaPayload 会话元信息（和 Java MetaPayload 一致）
type MetaPayload struct {
	ConversationID string `json:"conversationId"`
	TaskID         string `json:"taskId"`
}

// MessageDelta 增量消息（和 Java MessageDelta 一致）
type MessageDelta struct {
	Type  string `json:"type"`  // "response" | "think"
	Delta string `json:"delta"`
}

// CompletionPayload 完成信息（和 Java CompletionPayload 一致）
type CompletionPayload struct {
	MessageID string `json:"messageId,omitempty"`
	Title     string `json:"title,omitempty"`
}
