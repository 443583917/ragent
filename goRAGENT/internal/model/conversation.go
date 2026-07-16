package model

import "time"

// ChatMessage 对话消息（内存模型，非 DB）
type ChatMessage struct{ Role, Content string }

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

// ConversationSummaryDO t_conversation_summary
type ConversationSummaryDO struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID string    `gorm:"column:conversation_id"`
	UserID         string    `gorm:"column:user_id"`
	Content        string    `gorm:"column:content"`
	LastMessageID  string    `gorm:"column:last_message_id"`
	Deleted        int       `gorm:"column:deleted"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (ConversationSummaryDO) TableName() string { return "t_conversation_summary" }
