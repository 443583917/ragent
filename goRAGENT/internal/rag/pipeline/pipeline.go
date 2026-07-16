package pipeline

import (
	"context"
	"strings"
	"time"

	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"github.com/nageoffer/ragent/goRAGENT/internal/infra/llm"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/memory"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/prompt"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/retrieve"
	"github.com/nageoffer/ragent/goRAGENT/internal/rag/rewrite"
	"go.uber.org/zap"
)

type Ctx struct {
	Question, ConversationID, TaskID, UserID string
	DeepThinking bool
	History      []llm.Message
	RetrievalCtx *retrieve.RetrievalContext
}

type StreamCallback = llm.StreamCallback

// IntentResolver 意图解析抽象（*intent.Resolver 满足；nil 表示未启用）
type IntentResolver interface {
	ResolveAll(ctx context.Context, subQuestions []string) []retrieve.SubQuestionIntent
}

// QueryRewriter 查询改写抽象（*rewrite.Rewriter 满足；nil 表示未启用）
type QueryRewriter interface {
	RewriteWithSplit(ctx context.Context, question string, history []llm.Message) rewrite.RewriteResult
}

// GuidanceDetector 歧义引导抽象（*guidance.Detector 满足；nil 表示未启用）
// 返回引导话术，空串表示无需引导
type GuidanceDetector interface {
	Detect(ctx context.Context, question string, subs []retrieve.SubQuestionIntent) string
}

// MemoryService 对话记忆抽象（*memory.ConversationMemory 满足）
type MemoryService interface {
	LoadAndAppend(ctx context.Context, conversationID, userID string, msg memory.ChatMessage) []memory.ChatMessage
	// AppendAssistant 落库 assistant 消息并返回消息 ID（失败返回空串）
	AppendAssistant(ctx context.Context, conversationID, userID, content string) string
}

// SimplePipeline 精简版 Pipeline（env 配置驱动）
type SimplePipeline struct {
	cfg      *config.Config
	memory   MemoryService
	llm      *llm.ChatService
	prompts  *prompt.TemplateLoader
	retrieve *retrieve.RetrievalEngine
	rewriter QueryRewriter
	resolver IntentResolver
	guidance GuidanceDetector
}

func NewSimplePipeline(cfg *config.Config, mem MemoryService, llmSvc *llm.ChatService,
	prompts *prompt.TemplateLoader, re *retrieve.RetrievalEngine,
	rewriter QueryRewriter, resolver IntentResolver, gd GuidanceDetector) *SimplePipeline {
	return &SimplePipeline{cfg: cfg, memory: mem, llm: llmSvc, prompts: prompts,
		retrieve: re, rewriter: rewriter, resolver: resolver, guidance: gd}
}

// emptyRetrievalNotice 空检索短路提示文案（和 Java handleEmptyRetrieval 一致）
const emptyRetrievalNotice = "未检索到与问题相关的文档内容。"

func (p *SimplePipeline) Execute(ctx context.Context, pipeCtx *Ctx, cb StreamCallback) (func(), error) {
	// 0. 包装回调：累积回答 → 完成时落库 + 回填 messageId（短路场景复用）
	wrapped := &persistingCallback{inner: cb, mem: p.memory, cid: pipeCtx.ConversationID, uid: pipeCtx.UserID}

	// 1. 加载记忆
	msgs := p.memory.LoadAndAppend(ctx, pipeCtx.ConversationID, pipeCtx.UserID,
		memory.ChatMessage{Role: "user", Content: pipeCtx.Question})
	for _, m := range msgs {
		pipeCtx.History = append(pipeCtx.History, llm.Message{Role: m.Role, Content: m.Content})
	}

	// 2. 查询改写 + 子问题拆分（失败/未启用降级为原问题）
	rr := rewrite.RewriteResult{Rewritten: pipeCtx.Question, SubQuestions: []string{pipeCtx.Question}}
	if p.rewriter != nil {
		rr = p.rewriter.RewriteWithSplit(ctx, pipeCtx.Question, pipeCtx.History)
	}

	// 3. 意图解析（失败/未启用降级为空 → 自动回退全局检索）
	var subIntents []retrieve.SubQuestionIntent
	if p.resolver != nil {
		subIntents = p.resolver.ResolveAll(ctx, rr.SubQuestions)
	}

	// 4. 歧义引导短路（推送选项话术，照常落库，和 Java handleGuidance 一致）
	if p.guidance != nil {
		if text := p.guidance.Detect(ctx, rr.Rewritten, subIntents); text != "" {
			zap.L().Info("歧义引导短路", zap.String("question", rr.Rewritten))
			wrapped.OnContent(text)
			wrapped.OnComplete()
			return func() {}, nil
		}
	}

	// 5. SYSTEM 意图短路：跳过检索直答（和 Java handleSystemOnly 一致）
	if isSystemOnly(subIntents) {
		zap.L().Info("SYSTEM 意图直答", zap.String("question", pipeCtx.Question))
		return p.streamSystemResponse(ctx, pipeCtx, subIntents, wrapped), nil
	}

	// 6. 向量检索（意图定向 + 全局，embedding → Milvus → rerank）
	sc := &retrieve.SearchContext{
		OriginalQuestion:  pipeCtx.Question,
		RewrittenQuestion: rr.Rewritten,
		TopK:              p.cfg.RAG.TopK,
		Intents:           subIntents,
	}
	chunks, _ := p.retrieve.Search(ctx, sc)
	var kbText string
	if len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, c := range chunks { texts[i] = c.Text }
		kbText = strings.Join(texts, "\n")
	}
	pipeCtx.RetrievalCtx = &retrieve.RetrievalContext{KbContext: kbText, IsEmpty: kbText == ""}

	// 5. 空检索短路（和 Java handleEmptyRetrieval 一致：提示文案照常落库）
	if kbText == "" {
		zap.L().Info("空检索短路", zap.String("question", rr.Rewritten))
		wrapped.OnContent(emptyRetrievalNotice)
		wrapped.OnComplete()
		return func() {}, nil
	}

	// 6. 选模板：单意图且节点自带模板 → 覆盖；否则场景默认模板
	sysPrompt := singleIntentTemplate(subIntents)
	if sysPrompt == "" {
		sysPrompt, _ = p.prompts.Load("answer-chat-kb.st")
	}

	// 7. 组装消息
	messages := []llm.Message{{Role: "system", Content: sysPrompt}}
	messages = append(messages, pipeCtx.History...)
	evidence := "<documents>\n" + kbText + "\n</documents>"
	messages = append(messages, llm.Message{Role: "user", Content: evidence + "\n\n" + pipeCtx.Question})

	// 8. 流式回答（完成后 assistant 消息落库）
	temp := 0.0
	topp := 1.0
	req := llm.ChatRequest{Messages: messages, Temperature: &temp, TopP: &topp}
	zap.L().Info("流式回答", zap.String("question", rr.Rewritten),
		zap.Int("chunks", len(chunks)), zap.Int("subQuestions", len(rr.SubQuestions)))
	return p.llm.StreamChat(ctx, req, wrapped), nil
}

// singleIntentTemplate 全局仅命中单个意图且节点自带 promptTemplate 时返回该模板
// （和 Java RAGPromptService.planPrompt 一致）
func singleIntentTemplate(subs []retrieve.SubQuestionIntent) string {
	var only *retrieve.NodeRef
	count := 0
	for _, s := range subs {
		for _, ns := range s.NodeScores {
			count++
			only = ns.Node
		}
	}
	if count == 1 && only != nil {
		return strings.TrimSpace(only.PromptTemplate)
	}
	return ""
}

// isSystemOnly 所有子问题都恰好命中 1 个 SYSTEM 意图（和 Java IntentResolver.isSystemOnly 一致）
func isSystemOnly(subs []retrieve.SubQuestionIntent) bool {
	if len(subs) == 0 {
		return false
	}
	for _, s := range subs {
		if len(s.NodeScores) != 1 {
			return false
		}
		n := s.NodeScores[0].Node
		if n == nil || n.IsKB || n.IsMCP {
			return false
		}
	}
	return true
}

// streamSystemResponse SYSTEM 意图直答：system（节点模板可覆盖）+ 历史 + 原问题，
// 不带检索证据（和 Java streamSystemResponse 一致）
func (p *SimplePipeline) streamSystemResponse(ctx context.Context, pipeCtx *Ctx,
	subs []retrieve.SubQuestionIntent, cb StreamCallback) func() {
	sysPrompt := ""
	for _, s := range subs {
		for _, ns := range s.NodeScores {
			if ns.Node != nil && strings.TrimSpace(ns.Node.PromptTemplate) != "" {
				sysPrompt = strings.TrimSpace(ns.Node.PromptTemplate)
				break
			}
		}
		if sysPrompt != "" {
			break
		}
	}
	if sysPrompt == "" {
		sysPrompt, _ = p.prompts.Load("answer-chat-system.st")
	}

	messages := []llm.Message{{Role: "system", Content: sysPrompt}}
	messages = append(messages, pipeCtx.History...)
	messages = append(messages, llm.Message{Role: "user", Content: pipeCtx.Question})

	temp := 0.7
	return p.llm.StreamChat(ctx, llm.ChatRequest{Messages: messages, Temperature: &temp}, cb)
}

// persistingCallback 包装回调：累积回答内容，完成时落库 assistant 消息
// （和 Java StreamChatEventHandler.onComplete 对应；失败流不落库）
type persistingCallback struct {
	inner    StreamCallback
	mem      MemoryService
	cid, uid string
	buf      strings.Builder
}

func (p *persistingCallback) OnContent(chunk string) {
	p.buf.WriteString(chunk)
	p.inner.OnContent(chunk)
}
func (p *persistingCallback) OnThinking(chunk string) { p.inner.OnThinking(chunk) }
func (p *persistingCallback) OnError(err error)       { p.inner.OnError(err) }
// MessageIDSetter 可选接口：回调若实现，落库后回填消息 ID（finish 事件用）
type MessageIDSetter interface {
	SetMessageID(id string)
}

func (p *persistingCallback) OnComplete() {
	// 独立 ctx：完成时请求 context 可能随 handler 返回被取消
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id := p.mem.AppendAssistant(ctx, p.cid, p.uid, p.buf.String())
	if setter, ok := p.inner.(MessageIDSetter); ok && id != "" {
		setter.SetMessageID(id)
	}
	p.inner.OnComplete()
}
