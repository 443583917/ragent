package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Emitter 线程安全的 SSE 连接封装（和 Java SseEmitterSender 对应）
type Emitter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
	mu      sync.Mutex
}

// NewEmitter 从 Gin Context 创建 SSE 发送器
func NewEmitter(w http.ResponseWriter) (*Emitter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("ResponseWriter 不支持 Flusher 接口")
	}

	// 设置 SSE Headers（和 Spring SseEmitter 一致）
	w.Header().Set("Content-Type", "text/event-stream;charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &Emitter{
		w:       w,
		flusher: flusher,
	}, nil
}

// SendEvent 发送一个命名 SSE 事件
func (e *Emitter) SendEvent(eventName SSEEventType, data interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	// 标准 SSE 格式：event:xxx\ndata:xxx\n\n
	_, err = fmt.Fprintf(e.w, "event:%s\ndata:%s\n\n", eventName.Value(), string(jsonData))
	if err != nil {
		e.closed = true
		return
	}
	e.flusher.Flush()
}

// SendRaw 发送原始文本（用于 done 事件的 "[DONE]"）
func (e *Emitter) SendRaw(eventName SSEEventType, raw string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	_, err := fmt.Fprintf(e.w, "event:%s\ndata:%s\n\n", eventName.Value(), raw)
	if err != nil {
		e.closed = true
		return
	}
	e.flusher.Flush()
}

// SendChunked 按 code point 分块发送消息（和 Java sendChunked 一致）
func (e *Emitter) SendChunked(contentType string, text string, chunkSize int) {
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		e.SendEvent(EventMessage, MessageDelta{
			Type:  contentType,
			Delta: chunk,
		})
	}
}

// Complete 正常关闭 SSE 连接
func (e *Emitter) Complete() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
}

// Fail 异常关闭 SSE 连接
func (e *Emitter) Fail() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
}

// IsClosed 检查连接是否已关闭
func (e *Emitter) IsClosed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}
