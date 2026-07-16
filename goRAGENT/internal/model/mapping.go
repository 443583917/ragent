package model

import "time"

// TermMappingDO t_query_term_mapping 表映射
type TermMappingDO struct {
	ID         string    `gorm:"column:id;primaryKey" json:"id"`
	Domain     string    `gorm:"column:domain" json:"domain,omitempty"`
	SourceTerm string    `gorm:"column:source_term" json:"sourceTerm"`
	TargetTerm string    `gorm:"column:target_term" json:"targetTerm"`
	MatchType  int       `gorm:"column:match_type" json:"matchType"`
	Priority   int       `gorm:"column:priority" json:"priority"`
	Enabled    int       `gorm:"column:enabled" json:"enabled"`
	Remark     string    `gorm:"column:remark" json:"remark,omitempty"`
	CreateBy   string    `gorm:"column:create_by" json:"-"`
	UpdateBy   string    `gorm:"column:update_by" json:"-"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    int       `gorm:"column:deleted" json:"-"`
}

func (TermMappingDO) TableName() string { return "t_query_term_mapping" }

const MappingCacheKey  = "ragent:query-term:mappings"
const MappingCacheTTL  = 7 * 24 * time.Hour
const MatchTypeExact   = 1
