package memory

import (
	"context"
	"strconv"
	"strings"
	"time"

	"goRAGENT/internal/config"
	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/rag/prompt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ChatMessage struct{ Role, Content string }

// chatClient LLM 同步调用抽象（*llm.ChatService 满足）
type chatClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

// ConversationMemory 对话记忆（DB 存储，和 Java JdbcConversationMemoryStore 对应）
type ConversationMemory struct {
	cfg     *config.Config
	db      *gorm.DB
	rdb     *redis.Client
	llm     chatClient
	prompts *prompt.TemplateLoader
}

func NewConversationMemory(cfg *config.Config, db *gorm.DB, rdb *redis.Client,
	llmSvc chatClient, prompts *prompt.TemplateLoader) *ConversationMemory {
	if prompts == nil {
		prompts = prompt.NewTemplateLoader()
	}
	return &ConversationMemory{cfg: cfg, db: db, rdb: rdb, llm: llmSvc, prompts: prompts}
}

// LoadAndAppend 加载历史（摘要置顶 + 最近轮次，不含本轮），并把本轮 user 消息落库 + 会话 createOrUpdate。
// 任何 DB 故障降级为空历史，不阻断问答。
func (m *ConversationMemory) LoadAndAppend(ctx context.Context, conversationID, userID string, msg ChatMessage) []ChatMessage {
	if m.db == nil {
		return nil
	}
	history := m.loadHistory(ctx, conversationID, userID)
	// 摘要以 system 角色包裹置顶（和 Java attachSummary 一致）
	if summary, _ := m.loadLatestSummary(ctx, conversationID, userID); summary != "" {
		history = append([]ChatMessage{{Role: "system", Content: m.decorateSummary(summary)}}, history...)
	}
	m.appendMessage(ctx, conversationID, userID, msg.Role, msg.Content)
	m.createOrUpdateConversation(ctx, conversationID, userID, msg.Content)
	return history
}

// AppendAssistant 回答完成后落库 assistant 消息（和 Java StreamChatEventHandler.onComplete 对应），
// 返回消息 ID（用于 finish 事件回传前端做反馈），失败返回空串。落库后异步触发摘要压缩。
func (m *ConversationMemory) AppendAssistant(ctx context.Context, conversationID, userID, content string) string {
	if m.db == nil || strings.TrimSpace(content) == "" {
		return ""
	}
	id := m.appendMessage(ctx, conversationID, userID, "assistant", content)
	m.touchConversation(ctx, conversationID, userID)
	go m.maybeCompress(conversationID, userID)
	return id
}

func (m *ConversationMemory) loadHistory(ctx context.Context, conversationID, userID string) []ChatMessage {
	limit := m.cfg.Memory.HistoryKeepTurns * 2
	if limit <= 0 {
		return nil
	}
	var rows []ConversationMessageDO
	if err := m.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND deleted = 0", conversationID, userID).
		Order("id DESC").Limit(limit).
		Find(&rows).Error; err != nil {
		zap.L().Warn("加载对话历史失败，降级为空", zap.Error(err))
		return nil
	}
	return normalizeHistory(rows)
}

func (m *ConversationMemory) appendMessage(ctx context.Context, conversationID, userID, role, content string) string {
	row := ConversationMessageDO{
		ConversationID: conversationID, UserID: userID,
		Role: role, Content: content,
	}
	if err := m.db.WithContext(ctx).Create(&row).Error; err != nil {
		zap.L().Error("消息落库失败", zap.String("role", role), zap.Error(err))
		return ""
	}
	return strconv.FormatInt(row.ID, 10)
}

// createOrUpdateConversation 首条消息建会话（标题=问题截断），已存在则刷新最后时间
func (m *ConversationMemory) createOrUpdateConversation(ctx context.Context, conversationID, userID, question string) {
	var count int64
	if err := m.db.WithContext(ctx).Model(&ConversationDO{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Count(&count).Error; err != nil {
		zap.L().Warn("查询会话失败", zap.Error(err))
		return
	}
	if count == 0 {
		conv := ConversationDO{
			ConversationID: conversationID, UserID: userID,
			Title: truncateTitle(question, m.cfg.Memory.TitleMaxLength), LastTime: time.Now(),
		}
		if err := m.db.WithContext(ctx).Create(&conv).Error; err != nil {
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
					_ = m.db.WithContext(tctx).Model(&ConversationDO{}).
						Where("conversation_id = ? AND user_id = ?", conversationID, userID).
						Update("title", title).Error
				}
			}()
		}
		return
	}
	m.touchConversation(ctx, conversationID, userID)
}

func (m *ConversationMemory) touchConversation(ctx context.Context, conversationID, userID string) {
	if err := m.db.WithContext(ctx).Model(&ConversationDO{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Update("last_time", time.Now()).Error; err != nil {
		zap.L().Warn("更新会话时间失败", zap.Error(err))
	}
}

// ========== 纯函数 ==========

// normalizeHistory 输入按 id DESC 的最近 N 条 → 时间正序 + 去前导 assistant
// （窗口截断可能导致孤立 assistant，需去掉以保证 user/assistant 成对，和 Java loadHistory 一致）
func normalizeHistory(descRows []ConversationMessageDO) []ChatMessage {
	msgs := make([]ChatMessage, 0, len(descRows))
	for i := len(descRows) - 1; i >= 0; i-- { // 反转为正序
		msgs = append(msgs, ChatMessage{Role: descRows[i].Role, Content: descRows[i].Content})
	}
	start := 0
	for start < len(msgs) && msgs[start].Role != "user" {
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

// ========== DB 模型 ==========

// ConversationDO t_conversation
type ConversationDO struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID string    `gorm:"column:conversation_id"`
	UserID         string    `gorm:"column:user_id"`
	Title          string    `gorm:"column:title"`
	LastTime       time.Time `gorm:"column:last_time"`
	Deleted        int       `gorm:"column:deleted"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (ConversationDO) TableName() string { return "t_conversation" }

// ConversationMessageDO t_conversation_message
type ConversationMessageDO struct {
	ID               int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID   string    `gorm:"column:conversation_id"`
	UserID           string    `gorm:"column:user_id"`
	Role             string    `gorm:"column:role"`
	Content          string    `gorm:"column:content"`
	ThinkingContent  string    `gorm:"column:thinking_content"`
	ThinkingDuration *int      `gorm:"column:thinking_duration"`
	Vote             *int      `gorm:"column:vote"`
	Deleted          int       `gorm:"column:deleted"`
	CreateTime       time.Time `gorm:"column:create_time;autoCreateTime"`
}

func (ConversationMessageDO) TableName() string { return "t_conversation_message" }
