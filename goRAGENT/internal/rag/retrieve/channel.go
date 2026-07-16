package retrieve

import (
	"context"
)

// ========== 内联类型（避免循环依赖）==========

// SubQuestionIntent 子问题意图
type SubQuestionIntent struct {
	SubQuestion string
	NodeScores  []NodeScore
}

// NodeScore 节点评分
type NodeScore struct {
	Node *NodeRef
	Score float64
}

// NodeRef 轻量意图节点引用
type NodeRef struct {
	ID             string
	Name           string
	FullPath       string
	CollectionName string
	McpToolID      string
	PromptSnippet  string
	PromptTemplate string
	TopK           *int
	IsKB           bool
	IsMCP          bool
}

func (n *NodeRef) GetKind() string {
	if n == nil { return "" }
	if n.IsMCP { return "MCP" }
	if n.IsKB  { return "KB" }
	return "SYSTEM"
}

// ========== Chunk 检索结果 ==========

// RetrievedChunk 检索到的文档块（和 Java RetrievedChunk 一致）
type RetrievedChunk struct {
	ID       string  `json:"id"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ========== SearchChannel 接口（和 Java SearchChannel 对应）==========

// SearchChannelType 检索通道类型
type SearchChannelType string

const (
	ChannelVectorGlobal  SearchChannelType = "VECTOR_GLOBAL"
	ChannelIntentDirected SearchChannelType = "INTENT_DIRECTED"
	ChannelKeyword        SearchChannelType = "KEYWORD"
)

// SearchChannel 检索通道接口（和 Java 完全一致）
type SearchChannel interface {
	Name() string
	Priority() int
	IsEnabled(ctx context.Context, sc *SearchContext) bool
	Search(ctx context.Context, sc *SearchContext) (*ChannelResult, error)
	Type() SearchChannelType
}

// SearchContext 检索上下文（和 Java SearchContext 一致）
type SearchContext struct {
	OriginalQuestion  string                  `json:"originalQuestion"`
	RewrittenQuestion string                  `json:"rewrittenQuestion"`
	Intents           []SubQuestionIntent `json:"intents"`
	TopK              int                     `json:"topK"`
}

// ChannelResult 通道检索结果（和 Java SearchChannelResult 一致）
type ChannelResult struct {
	ChannelType SearchChannelType `json:"channelType"`
	ChannelName string            `json:"channelName"`
	Chunks      []RetrievedChunk   `json:"chunks"`
	Confidence  float64            `json:"confidence,omitempty"`
	LatencyMs   int64              `json:"latencyMs,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
}

// KBIntents 提取 KB 意图和最高分
func (sc *SearchContext) KBIntents() []NodeScore {
	var kb []NodeScore
	for _, si := range sc.Intents {
		for _, ns := range si.NodeScores {
			if ns.Node != nil && ns.Node.IsKB {
				kb = append(kb, ns)
			}
		}
	}
	return kb
}

// MaxScore 最高意图分数
func (sc *SearchContext) MaxScore() float64 {
	max := 0.0
	for _, si := range sc.Intents {
		for _, ns := range si.NodeScores {
			if ns.Score > max {
				max = ns.Score
			}
		}
	}
	return max
}
