package pipeline

import (
	"sync"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/framework/sse"
	"goRAGENT/internal/framework/userctx"
	"go.uber.org/zap"
)

type ChatHandler struct {
	cfg      *config.Config
	pipeline *SimplePipeline
}

func NewSimpleChatHandler(cfg *config.Config, pl *SimplePipeline) *ChatHandler {
	return &ChatHandler{cfg: cfg, pipeline: pl}
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

	pipeCtx := &Ctx{Question: q, ConversationID: cid, TaskID: taskID, UserID: uid}
	done := make(chan struct{})
	cb := &sseCallback{emitter: emitter, chunkSize: 1, done: done}

	cancel, err := h.pipeline.Execute(c.Request.Context(), pipeCtx, cb)
	if err != nil {
		zap.L().Error("Pipeline 失败", zap.Error(err))
		emitter.SendRaw(sse.EventDone, "[DONE]")
		emitter.Complete()
		return
	}

	// Go net/http 是同步模型：handler 返回即请求结束（context 被取消、
	// ResponseWriter 被回收），必须阻塞到流式输出完成或客户端断连
	select {
	case <-done: // OnComplete / OnError
	case <-c.Request.Context().Done(): // 客户端断连
		cancel()
		emitter.Complete()
	}
}

func (h *ChatHandler) StopTask(c *gin.Context) {
	c.JSON(200, gin.H{"code":"0"})
}

type sseCallback struct {
	emitter   *sse.Emitter
	chunkSize int
	done      chan struct{}
	doneOnce  sync.Once
	messageID string
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
