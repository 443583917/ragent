package pipeline

import (
	"context"
	"sync"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/framework/sse"
	"goRAGENT/internal/framework/userctx"
	"goRAGENT/internal/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ChatHandler struct {
	cfg       *config.Config
	pipeline  *SimplePipeline
	db        *gorm.DB
	mu        sync.Mutex
	taskCtxs  map[string]context.CancelFunc // taskID → cancel
}

func NewSimpleChatHandler(cfg *config.Config, pl *SimplePipeline, db *gorm.DB) *ChatHandler {
	return &ChatHandler{cfg: cfg, pipeline: pl, db: db, taskCtxs: make(map[string]context.CancelFunc)}
}

func (h *ChatHandler) StreamChat(c *gin.Context) {
	q := c.Query("question")
	if q == "" { c.JSON(400, gin.H{"code":"B000001","message":"question 参数不能为空"}); return }
	cid := c.Query("conversationId")
	if cid == "" { cid = snowflake.NextID() }

	uid := userctx.GetUserID(c.Request.Context())
	if uid == "" { c.JSON(401, gin.H{"code":"A000001","message":"未登录"}); return }

	emitter, err := sse.NewEmitter(c.Writer)
	if err != nil { return }

	taskID := snowflake.NextID()
	emitter.SendEvent(sse.EventMeta, sse.MetaPayload{ConversationID: cid, TaskID: taskID})

	// Trace: write run record (skip if no db, e.g. in tests)
	if h.db != nil {
		h.db.Create(&rag.TraceRunDO{
			RunID: taskID, ConversationID: cid, UserID: uid,
			Question: q, Status: "RUNNING",
		})
	}

	pipeCtx := &Ctx{Question: q, ConversationID: cid, TaskID: taskID, UserID: uid}
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

	// 阻塞到流式输出完成或客户端断连
	select {
	case <-done:
		if h.db != nil {
			h.db.Model(&rag.TraceRunDO{}).Where("run_id = ?", taskID).Update("status", "DONE")
		}
	case <-c.Request.Context().Done():
		cancel()
		cancelFn()
		emitter.Complete()
		if h.db != nil {
			h.db.Model(&rag.TraceRunDO{}).Where("run_id = ?", taskID).Updates(map[string]any{
				"status": "CANCELLED", "error_message": "客户端断连",
			})
		}
	}

	h.mu.Lock()
	delete(h.taskCtxs, taskID)
	h.mu.Unlock()
}

func (h *ChatHandler) StopTask(c *gin.Context) {
	taskID := c.Query("taskId")
	if taskID == "" {
		c.JSON(200, gin.H{"code":"0"})
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
		if h.db != nil {
			h.db.Model(&rag.TraceRunDO{}).Where("run_id = ?", taskID).Updates(map[string]any{
				"status": "CANCELLED", "error_message": "用户取消",
			})
		}
	}
	c.JSON(200, gin.H{"code":"0"})
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

// SetMessageID 落库后回填消息 ID（pipeline.MessageIDSetter）
func (c *sseCallback) SetMessageID(id string) { c.messageID = id }

func (c *sseCallback) finish() {
	c.doneOnce.Do(func() { close(c.done) })
}

func (c *sseCallback) OnContent(chunk string) {
	if c.chunkSize <= 1 {
		c.emitter.SendEvent(sse.EventMessage, sse.MessageDelta{Type:"response", Delta:chunk})
	} else {
		c.emitter.SendChunked("response", chunk, c.chunkSize)
	}
}
func (c *sseCallback) OnThinking(chunk string) {
	c.emitter.SendEvent(sse.EventMessage, sse.MessageDelta{Type:"think", Delta:chunk})
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
