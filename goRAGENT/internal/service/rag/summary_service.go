package rag

import (
	"context"
	"strconv"
	"strings"
	"time"

	"goRAGENT/internal/model"
	"goRAGENT/pkg/llm"
	"go.uber.org/zap"
)

const (
	summaryPromptPath = "conversation-summary.st"
	titlePromptPath   = "conversation-title.st"
	contextFormatPath = "context-format.st"

	// summaryLockPrefix 分布式锁前缀（和 Java SUMMARY_LOCK_PREFIX 一致）
	summaryLockPrefix = "ragent:memory:summary:lock:"
	summaryLockTTL    = 30 * time.Second
)

// ConversationSummaryDO moved to internal/model/conversation.go

// ========== 纯函数 ==========

// summaryCutoffIndex 摘要截止点 = 窗口中位（和 Java (size-1)/2 一致，摘要覆盖约一半窗口）
func summaryCutoffIndex(n int) int { return (n - 1) / 2 }

// shouldSkipSummary 上次摘要已覆盖当前窗口起点则跳过
func shouldSkipSummary(afterID, historyStartID int64) bool {
	return afterID > 0 && afterID >= historyStartID
}

// buildSummaryMessages 摘要 LLM 消息序列：system → [历史摘要注入] → 对话原文 → 合并指令
// （和 Java summarizeMessages 一致）
func buildSummaryMessages(sysPrompt string, msgs []model.ConversationMessageDO, existing string, maxChars int) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: sysPrompt}}
	if strings.TrimSpace(existing) != "" {
		messages = append(messages, llm.Message{Role: "assistant",
			Content: "历史摘要（仅用于合并去重，不得作为事实新增来源；若与本轮对话冲突，以本轮对话为准）：\n" + strings.TrimSpace(existing)})
	}
	for _, m := range msgs {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, llm.Message{Role: "user",
		Content: "合并以上对话与历史摘要，去重后输出更新摘要。要求：严格≤" + strconv.Itoa(maxChars) + "字符；仅一行。"})
	return messages
}

// ========== 摘要加载与包装 ==========

// decorateSummary 用 summary-wrapper section 包裹摘要内容
func (m *ConversationMemory) decorateSummary(content string) string {
	wrapped, err := m.prompts.RenderSection(contextFormatPath, "summary-wrapper",
		map[string]string{"content": content})
	if err != nil {
		return "<conversation-summary>\n" + content + "\n</conversation-summary>"
	}
	return wrapped
}

// loadLatestSummary 最新一条摘要（content, lastMessageID）；无摘要返回 "", 0
func (m *ConversationMemory) loadLatestSummary(ctx context.Context, conversationID, userID string) (string, int64) {
	if m.db == nil {
		return "", 0
	}
	var row model.ConversationSummaryDO
	err := m.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND deleted = 0", conversationID, userID).
		Order("id DESC").Limit(1).
		Take(&row).Error
	if err != nil {
		return "", 0
	}
	lastID, _ := strconv.ParseInt(row.LastMessageID, 10, 64)
	return row.Content, lastID
}

// ========== 摘要压缩（assistant 落库后异步触发，和 Java doCompressIfNeeded 一致）==========

func (m *ConversationMemory) maybeCompress(conversationID, userID string) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("摘要压缩 panic", zap.Any("recover", r))
		}
	}()
	mc := m.cfg.Memory
	if !mc.SummaryEnabled || mc.SummaryStartTurns <= 0 || mc.HistoryKeepTurns <= 0 ||
		mc.SummaryStartTurns <= mc.HistoryKeepTurns || m.db == nil || m.llm == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Redis SETNX 分布式锁（拿不到立即放弃，不等待）
	if m.rdb != nil {
		lockKey := summaryLockPrefix + strings.TrimSpace(userID) + ":" + strings.TrimSpace(conversationID)
		ok, err := m.rdb.SetNX(ctx, lockKey, "1", summaryLockTTL).Result()
		if err != nil || !ok {
			return
		}
		defer m.rdb.Del(context.Background(), lockKey)
	}

	// 用户消息总数达到触发轮数
	var total int64
	if err := m.db.WithContext(ctx).Model(&model.ConversationMessageDO{}).
		Where("conversation_id = ? AND user_id = ? AND role = 'user' AND deleted = 0", conversationID, userID).
		Count(&total).Error; err != nil || total < int64(mc.SummaryStartTurns) {
		return
	}

	// 最近 keepTurns 条 user 消息窗口（DESC）
	var window []model.ConversationMessageDO
	if err := m.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND role = 'user' AND deleted = 0", conversationID, userID).
		Order("id DESC").Limit(mc.HistoryKeepTurns).
		Find(&window).Error; err != nil || len(window) == 0 {
		return
	}
	historyStartID := window[len(window)-1].ID

	existing, afterID := m.loadLatestSummary(ctx, conversationID, userID)
	if shouldSkipSummary(afterID, historyStartID) {
		return
	}
	cutoffID := window[summaryCutoffIndex(len(window))].ID

	// 待摘要消息：afterID < id < cutoffID
	var toSummarize []model.ConversationMessageDO
	if err := m.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND role IN ('user','assistant') AND deleted = 0 AND id > ? AND id < ?",
			conversationID, userID, afterID, cutoffID).
		Order("id ASC").
		Find(&toSummarize).Error; err != nil || len(toSummarize) == 0 {
		return
	}
	lastMessageID := toSummarize[len(toSummarize)-1].ID

	// LLM 生成摘要
	tpl, err := m.prompts.Load(summaryPromptPath)
	if err != nil {
		return
	}
	sysPrompt := strings.ReplaceAll(tpl, "{summary_max_chars}", strconv.Itoa(mc.SummaryMaxChars))
	temp, topP := 0.3, 0.9
	content, err := m.llm.Chat(ctx, llm.ChatRequest{
		Messages:    buildSummaryMessages(sysPrompt, toSummarize, existing, mc.SummaryMaxChars),
		Temperature: &temp, TopP: &topP,
	})
	if err != nil || strings.TrimSpace(content) == "" {
		zap.L().Warn("摘要生成失败", zap.Error(err))
		return
	}

	// 插入新行（和 Java 一致，不更新旧行）
	row := model.ConversationSummaryDO{
		ConversationID: conversationID, UserID: userID,
		Content: strings.TrimSpace(content), LastMessageID: strconv.FormatInt(lastMessageID, 10),
	}
	if err := m.db.WithContext(ctx).Create(&row).Error; err != nil {
		zap.L().Error("摘要落库失败", zap.Error(err))
		return
	}
	zap.L().Info("对话摘要已压缩", zap.String("conversationId", conversationID),
		zap.Int64("lastMessageId", lastMessageID), zap.Int("chars", len([]rune(row.Content))))
}

// ========== 会话标题（新会话异步生成，和 Java ConversationTitleGenerator 一致）==========

func (m *ConversationMemory) generateTitle(ctx context.Context, question string) string {
	fallback := truncateTitle(question, m.cfg.Memory.TitleMaxLength)
	if m.llm == nil {
		return fallback
	}
	tpl, err := m.prompts.Load(titlePromptPath)
	if err != nil {
		return fallback
	}
	promptText := strings.ReplaceAll(tpl, "{question}", question)
	promptText = strings.ReplaceAll(promptText, "{title_max_chars}", strconv.Itoa(m.cfg.Memory.TitleMaxLength))

	temp, topP := 0.7, 0.3
	raw, err := m.llm.Chat(ctx, llm.ChatRequest{
		Messages:    []llm.Message{{Role: "user", Content: promptText}},
		Temperature: &temp, TopP: &topP,
	})
	if err != nil {
		zap.L().Warn("生成会话标题失败，使用截断标题", zap.Error(err))
		return fallback
	}
	title := cleanTitle(raw, m.cfg.Memory.TitleMaxLength)
	if title == "" {
		return fallback
	}
	return title
}

// cleanTitle 取首行、去引号、按 rune 截断
func cleanTitle(raw string, max int) string {
	title := strings.TrimSpace(raw)
	if idx := strings.IndexAny(title, "\r\n"); idx >= 0 {
		title = title[:idx]
	}
	title = strings.Trim(title, "\"“”'‘’「」")
	return truncateTitle(title, max)
}
