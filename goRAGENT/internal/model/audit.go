package model

import "time"

// BizChangeLogDO t_biz_change_log 业务变更审计日志
type BizChangeLogDO struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	EntityType     string    `gorm:"column:entity_type"`
	EntityID       string    `gorm:"column:entity_id"`
	Action         string    `gorm:"column:action"`
	Operator       string    `gorm:"column:operator"`
	BeforeSnapshot string    `gorm:"column:before_snapshot"`
	AfterSnapshot  string    `gorm:"column:after_snapshot"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime"`
}

func (BizChangeLogDO) TableName() string { return "t_biz_change_log" }

// AuditLogFilter 审计日志列表过滤条件（字段对照 listBizChangeLogs 的查询条件）。
type AuditLogFilter struct {
	EntityType string // 非空时精确匹配 entity_type
}
