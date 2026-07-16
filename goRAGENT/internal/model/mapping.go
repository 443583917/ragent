package model

import "time"

// TermMappingDO t_query_term_mapping 表映射
type TermMappingDO struct {
	ID         string    `gorm:"column:id;primaryKey"`
	Domain     string    `gorm:"column:domain"`
	SourceTerm string    `gorm:"column:source_term"`
	TargetTerm string    `gorm:"column:target_term"`
	MatchType  int       `gorm:"column:match_type"`
	Priority   int       `gorm:"column:priority"`
	Enabled    int       `gorm:"column:enabled"`
	Remark     string    `gorm:"column:remark"`
	CreateBy   string    `gorm:"column:create_by"`
	UpdateBy   string    `gorm:"column:update_by"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime"`
	Deleted    int       `gorm:"column:deleted"`
}

func (TermMappingDO) TableName() string { return "t_query_term_mapping" }

const MappingCacheKey  = "ragent:query-term:mappings"
const MappingCacheTTL  = 7 * 24 * time.Hour
const MatchTypeExact   = 1
