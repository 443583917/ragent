package model

// BizChangeLogVO 审计日志列表/详情 VO。
type BizChangeLogVO struct {
	ID             int64  `json:"id"`
	EntityType     string `json:"entityType"`
	EntityID       string `json:"entityId"`
	Action         string `json:"action"`
	Operator       string `json:"operator"`
	BeforeSnapshot string `json:"beforeSnapshot,omitempty"`
	AfterSnapshot  string `json:"afterSnapshot,omitempty"`
	CreateTime     string `json:"createTime"`
}
