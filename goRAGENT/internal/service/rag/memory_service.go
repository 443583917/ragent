package rag

import (
	"context"
	"strconv"
	"strings"
	"time"

	"goRAGENT/internal/config"
	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/pkg/prompt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// chatClient LLM 同步调用抽象（*llm.ChatService 满足）

// ConversationMemory 对话记忆（DB 存储，对话记忆，DB 存储）
type ConversationMemory struct {
	cfg         *config.Config
	convRepo    repository.ConversationRepository
	msgRepo     repository.MessageRepository
	summaryRepo repository.SummaryRepository
	rdb         *redis.Client
	llm         chatClient
	prompts     *prompt.TemplateLoader
}

func NewConversationMemory(cfg *config.Config, convRepo repository.ConversationRepository,
	msgRepo repository.MessageRepository, summaryRepo repository.SummaryRepository,
	rdb *redis.Client, llmSvc chatClient, prompts *prompt.TemplateLoader) *ConversationMemory {
	if prompts == nil {
		prompts = prompt.NewTemplateLoader()
	}
	return &ConversationMemory{cfg: cfg, convRepo: convRepo, msgRepo: msgRepo, summaryRepo: summaryRepo,
		rdb: rdb, llm: llmSvc, prompts: prompts}
}

// LoadAndAppend 加载历史（摘要置顶 + 最近轮次，不含本轮），并把本轮 user 消息落库 + 会话 createOrUpdate。
// 任何 DB 故障降级为空历史，不阻断问答。
func (m *ConversationMemory) LoadAndAppend(ctx context.Context, conversationID, userID string, msg model.ChatMessage) []model.ChatMessage {
	if m.msgRepo == nil {
		return nil
	}
	history := m.loadHistory(ctx, conversationID, userID)
	// 摘要以 system 角色包裹置顶
	if summary, _ := m.loadLatestSummary(ctx, conversationID, userID); summary != "" {
		history = append([]model.ChatMessage{{Role: "system", Content: m.decorateSummary(summary)}}, history...)
	}
	m.appendMessage(ctx, conversationID, userID, msg.Role, msg.Content)
	m.createOrUpdateConversation(ctx, conversationID, userID, msg.Content)
	return history
}

// AppendAssistant 回答完成后落库 assistant 消息（回答完成后落库 assistant 消息），
// 返回消息 ID（用于 finish 事件回传前端做反馈），失败返回空串。落库后异步触发摘要压缩。
func (m *ConversationMemory) AppendAssistant(ctx context.Context, conversationID, userID, content string) string {
	if m.msgRepo == nil || strings.TrimSpace(content) == "" {
		return ""
	}
	id := m.appendMessage(ctx, conversationID, userID, model.MsgRoleAssistant, content)
	m.touchConversation(ctx, conversationID, userID)
	go m.maybeCompress(conversationID, userID)
	return id
}

func (m *ConversationMemory) loadHistory(ctx context.Context, conversationID, userID string) []model.ChatMessage {
	limit := m.cfg.Memory.HistoryKeepTurns * 2
	if limit <= 0 {
		return nil
	}
	rows, err := m.msgRepo.ListRecentForUser(ctx, conversationID, userID, limit)
	if err != nil {
		zap.L().Warn("加载对话历史失败，降级为空", zap.Error(err))
		return nil
	}
	return normalizeHistory(rows)
}

func (m *ConversationMemory) appendMessage(ctx context.Context, conversationID, userID, role, content string) string {
	row := model.ConversationMessageDO{
		ConversationID: conversationID, UserID: userID,
		Role: role, Content: content,
	}
	if err := m.msgRepo.Create(ctx, &row); err != nil {
		zap.L().Error("消息落库失败", zap.String("role", role), zap.Error(err))
		return ""
	}
	return strconv.FormatInt(row.ID, 10)
}

// createOrUpdateConversation 首条消息建会话（标题=问题截断），已存在则刷新最后时间
func (m *ConversationMemory) createOrUpdateConversation(ctx context.Context, conversationID, userID, question string) {
	exists, err := m.convRepo.ExistsForUser(ctx, conversationID, userID)
	if err != nil {
		zap.L().Warn("查询会话失败", zap.Error(err))
		return
	}
	if !exists {
		conv := model.ConversationDO{
			ConversationID: conversationID, UserID: userID,
			Title: truncateTitle(question, m.cfg.Memory.TitleMaxLength), LastTime: time.Now(),
		}
		if err := m.convRepo.Create(ctx, &conv); err != nil {
			zap.L().Warn("创建会话失败", zap.Error(err))
			return
		}
		// 异步 LLM 生成更好的标题（失败保持截断标题）
		if m.llm != nil {
			go func() {
				defer func() { _ = recover() }()
				tctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if title := m.generateTitle(tctx, question); title != "" && title != conv.Title {
					_ = m.convRepo.UpdateFieldsForUser(tctx, conversationID, userID,
						map[string]any{"title": title})
				}
			}()
		}
		return
	}
	m.touchConversation(ctx, conversationID, userID)
}

func (m *ConversationMemory) touchConversation(ctx context.Context, conversationID, userID string) {
	if err := m.convRepo.UpdateFieldsForUser(ctx, conversationID, userID,
		map[string]any{"last_time": time.Now()}); err != nil {
		zap.L().Warn("更新会话时间失败", zap.Error(err))
	}
}

// ========== 纯函数 ==========

// normalizeHistory 输入按 id DESC 的最近 N 条 → 时间正序 + 去前导 assistant
// （窗口截断可能导致孤立 assistant，需去掉以保证 user/assistant 成对，）
func normalizeHistory(descRows []model.ConversationMessageDO) []model.ChatMessage {
	msgs := make([]model.ChatMessage, 0, len(descRows))
	for i := len(descRows) - 1; i >= 0; i-- { // 反转为正序
		msgs = append(msgs, model.ChatMessage{Role: descRows[i].Role, Content: descRows[i].Content})
	}
	start := 0
	for start < len(msgs) && msgs[start].Role != model.MsgRoleUser {
		start++
	}
	return msgs[start:]
}

// truncateTitle 按 rune 截断标题（中文安全），去首尾空白
func truncateTitle(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
