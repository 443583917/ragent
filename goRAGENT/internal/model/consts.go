package model

// 向量库相关常量
const (
	// DefaultVectorDimension 默认向量维度（BGE-M3 / OpenAI ada 兼容维度）
	DefaultVectorDimension = 1536
	// KBCollectionPrefix 知识库对应 Milvus Collection 名前缀
	KBCollectionPrefix = "kb_"
)

// TraceRun 状态（对照 t_rag_trace_run.status 既有取值）
const (
	TraceStatusRunning   = "RUNNING"
	TraceStatusSuccess   = "SUCCESS"
	TraceStatusFailed    = "FAILED"
	TraceStatusCancelled = "CANCELLED"
	TraceStatusEmpty     = "EMPTY"
)

// 用户角色
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// 消息角色（t_conversation_message.role）
const (
	MsgRoleUser      = "user"
	MsgRoleAssistant = "assistant"
)

// DocumentPreviewMaxRunes 文档预览截断长度（原 document.go 硬编码 5000）
const DocumentPreviewMaxRunes = 5000

// 软删除标记
const (
	NotDeleted = 0
	Deleted    = 1
)

// 启用标记
const (
	Disabled = 0
	Enabled  = 1
)
