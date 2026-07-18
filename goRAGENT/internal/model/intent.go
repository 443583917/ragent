package model

import (
	"encoding/json"
	"time"
)

// IntentKind 意图类型（0=KB, 1=SYSTEM, 2=MCP）
type IntentKind int

const (
	KindKB     IntentKind = iota // KB
	KindSystem                   // SYSTEM
	KindMCP                      // MCP
)

func (k IntentKind) String() string {
	switch k {
	case KindMCP:
		return "MCP"
	case KindSystem:
		return "SYSTEM"
	default:
		return "KB"
	}
}

// IntentNode 意图树内存模型（ID = intent_code，内存模型）
type IntentNode struct {
	ID                  string        `json:"id"`
	KbID                string        `json:"kbId,omitempty"`
	Name                string        `json:"name"`
	Description         string        `json:"description,omitempty"`
	Level               int           `json:"level"`
	ParentID            string        `json:"parentId,omitempty"`
	Examples            []string      `json:"examples,omitempty"`
	Children            []*IntentNode `json:"children,omitempty"`
	FullPath            string        `json:"fullPath,omitempty"`
	Kind                IntentKind    `json:"kind"`
	CollectionName      string        `json:"collectionName,omitempty"`
	McpToolID           string        `json:"mcpToolId,omitempty"`
	TopK                *int          `json:"topK,omitempty"`
	PromptSnippet       string        `json:"promptSnippet,omitempty"`
	PromptTemplate      string        `json:"promptTemplate,omitempty"`
	ParamPromptTemplate string        `json:"paramPromptTemplate,omitempty"`
}

// IntentNodeDO t_intent_node 表映射（）
type IntentNodeDO struct {
	ID                  string    `gorm:"column:id;primaryKey"`
	KbID                string    `gorm:"column:kb_id"`
	IntentCode          string    `gorm:"column:intent_code"`
	Name                string    `gorm:"column:name"`
	Level               int       `gorm:"column:level"`
	ParentCode          string    `gorm:"column:parent_code"`
	Description         string    `gorm:"column:description"`
	Examples            string    `gorm:"column:examples"` // JSON 数组字符串
	CollectionName      string    `gorm:"column:collection_name"`
	TopK                *int      `gorm:"column:top_k"`
	McpToolID           string    `gorm:"column:mcp_tool_id"`
	Kind                int       `gorm:"column:kind"`
	PromptSnippet       string    `gorm:"column:prompt_snippet"`
	PromptTemplate      string    `gorm:"column:prompt_template"`
	ParamPromptTemplate string    `gorm:"column:param_prompt_template"`
	SortOrder           int       `gorm:"column:sort_order"`
	Enabled             int       `gorm:"column:enabled"`
	CreateBy            string    `gorm:"column:create_by"`
	UpdateBy            string    `gorm:"column:update_by"`
	CreateTime          time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime          time.Time `gorm:"column:update_time;autoUpdateTime"`
	Deleted             int       `gorm:"column:deleted"`
}

func (IntentNodeDO) TableName() string { return "t_intent_node" }

// toNode DO → 内存模型（ID 取业务标识 intent_code，）
func (d IntentNodeDO) ToNode() *IntentNode {
	var examples []string
	if d.Examples != "" {
		// 非法 JSON 容错为空
		_ = json.Unmarshal([]byte(d.Examples), &examples)
	}
	return &IntentNode{
		ID:                  d.IntentCode,
		KbID:                d.KbID,
		Name:                d.Name,
		Description:         d.Description,
		Level:               d.Level,
		ParentID:            d.ParentCode,
		Examples:            examples,
		Kind:                IntentKind(d.Kind),
		CollectionName:      d.CollectionName,
		McpToolID:           d.McpToolID,
		TopK:                d.TopK,
		PromptSnippet:       d.PromptSnippet,
		PromptTemplate:      d.PromptTemplate,
		ParamPromptTemplate: d.ParamPromptTemplate,
	}
}
