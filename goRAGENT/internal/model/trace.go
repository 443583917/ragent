package model

import "time"

// TraceRunDO t_rag_trace_run
type TraceRunDO struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunID          string    `gorm:"column:run_id"`
	ConversationID string    `gorm:"column:conversation_id"`
	UserID         string    `gorm:"column:user_id"`
	Question       string    `gorm:"column:question"`
	Status         string    `gorm:"column:status"`
	ErrorMessage   string    `gorm:"column:error_message"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (TraceRunDO) TableName() string { return "t_rag_trace_run" }

// TraceNodeDO t_rag_trace_node
type TraceNodeDO struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunID        string    `gorm:"column:run_id"`
	ParentNodeID string    `gorm:"column:parent_node_id"`
	NodeName     string    `gorm:"column:node_name"`
	NodeType     string    `gorm:"column:node_type"`
	Input        string    `gorm:"column:input"`
	Output       string    `gorm:"column:output"`
	DurationMs   int64     `gorm:"column:duration_ms"`
	ErrorMessage string    `gorm:"column:error_message"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime"`
}

func (TraceNodeDO) TableName() string { return "t_rag_trace_node" }
