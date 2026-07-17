package chat

import (
	"context"
	"sync"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
	"goRAGENT/internal/service/rag"
	"goRAGENT/pkg/snowflake"
	"goRAGENT/pkg/sse"
	"go.uber.org/zap"
)

type ChatHandler struct {
	cfg      *config.Config
	pipeline *rag.SimplePipeline
	recorder rag.TraceRecorder
	mu       sync.Mutex
	taskCtxs map[string]context.CancelFunc
}

func NewSimpleChatHandler(cfg *config.Config, pl *rag.SimplePipeline, recorder rag.TraceRecorder) *ChatHandler {
	return &ChatHandler{cfg: cfg, pipeline: pl, recorder: recorder, taskCtxs: make(map[string]context.CancelFunc)}
}

func (h *ChatHandler) StreamChat(c *gin.Context) {
	q := c.Query("question")
	if q == "" {
		c.JSON(400, gin.H{"code": "B000001", "message": "question 参数不能为空"})
		return
	}
	cid := c.Query("conversationId")
	if cid == "" {
		cid = snowflake.NextID()
	}

	uid := middleware.GetUserID(c.Request.Context())
	if uid == "" {
		c.JSON(401, gin.H{"code": "A000001", "message": "未登录"})
		return
	}

	emitter, err := sse.NewEmitter(c.Writer)
	if err != nil {
		return
	}

	taskID := snowflake.NextID()
	emitter.SendEvent(sse.EventMeta, sse.MetaPayload{ConversationID: cid, TaskID: taskID})

	if h.recorder != nil {
		if err := h.recorder.StartRun(c.Request.Context(), taskID, cid, uid, q); err != nil {
			zap.L().Error("创建追踪记录失败", zap.Error(err))
		}
	}

	pipeCtx := &rag.Ctx{Question: q, ConversationID: cid, TaskID: taskID, UserID: uid}
	done := make(chan struct{})
	cb := &sseCallback{emitter: emitter, chunkSize: 1, done: done, h: h, taskID: taskID}

	ctx, cancel := context.WithCancel(c.Request.Context())
	h.mu.Lock()
	h.taskCtxs[taskID] = cancel
	h.mu.Unlock()

	cancelFn, err := h.pipeline.Execute(ctx, pipeCtx, cb)
	if err != nil {
		zap.L().Error("Pipeline 失败", zap.Error(err))
		emitter.SendRaw(sse.EventDone, "[DONE]")
		emitter.Complete()
		h.mu.Lock()
		delete(h.taskCtxs, taskID)
		h.mu.Unlock()
		return
	}

	select {
	case <-done:
		if h.recorder != nil {
			if err := h.recorder.FinishRun(context.Background(), taskID, model.TraceStatusSuccess); err != nil {
				zap.L().Error("更新追踪状态失败", zap.Error(err))
			}
		}
	case <-c.Request.Context().Done():
		cancel()
		cancelFn()
		emitter.Complete()
		if h.recorder != nil {
			if err := h.recorder.CancelByTaskID(context.Background(), taskID, rag.TraceErrClientDisconnect); err != nil {
				zap.L().Error("取消追踪记录失败", zap.Error(err))
			}
		}
	}

	h.mu.Lock()
	delete(h.taskCtxs, taskID)
	h.mu.Unlock()
}

func (h *ChatHandler) StopTask(c *gin.Context) {
	taskID := c.Query("taskId")
	if taskID == "" {
		c.JSON(200, gin.H{"code": "0"})
		return
	}

	h.mu.Lock()
	cancel, ok := h.taskCtxs[taskID]
	if ok {
		delete(h.taskCtxs, taskID)
	}
	h.mu.Unlock()

	if ok {
		cancel()
		if h.recorder != nil {
			if err := h.recorder.CancelByTaskID(context.Background(), taskID, rag.TraceErrUserCancel); err != nil {
				zap.L().Error("取消追踪记录失败", zap.Error(err))
			}
		}
	}
	c.JSON(200, gin.H{"code": "0"})
}

type sseCallback struct {
	emitter   *sse.Emitter
	chunkSize int
	done      chan struct{}
	doneOnce  sync.Once
	messageID string
	h         *ChatHandler
	taskID    string
}

func (c *sseCallback) SetMessageID(id string) { c.messageID = id }

func (c *sseCallback) finish() {
	c.doneOnce.Do(func() { close(c.done) })
}

func (c *sseCallback) OnContent(chunk string) {
	if c.chunkSize <= 1 {
		c.emitter.SendEvent(sse.EventMessage, sse.MessageDelta{Type: "response", Delta: chunk})
	} else {
		c.emitter.SendChunked("response", chunk, c.chunkSize)
	}
}
func (c *sseCallback) OnThinking(chunk string) {
	c.emitter.SendEvent(sse.EventMessage, sse.MessageDelta{Type: "think", Delta: chunk})
}
func (c *sseCallback) OnComplete() {
	c.emitter.SendEvent(sse.EventFinish, sse.CompletionPayload{MessageID: c.messageID})
	c.emitter.SendRaw(sse.EventDone, "[DONE]")
	c.emitter.Complete()
	c.finish()
}
func (c *sseCallback) OnError(err error) {
	zap.L().Error("流式错误", zap.Error(err))
	c.emitter.SendRaw(sse.EventDone, "[DONE]")
	c.emitter.Fail()
	c.finish()
}
