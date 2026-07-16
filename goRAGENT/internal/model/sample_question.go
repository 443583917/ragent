package model

import "time"

// SampleQuestionDO t_sample_question
type SampleQuestionDO struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Title       string    `gorm:"column:title"`
	Description string    `gorm:"column:description"`
	Question    string    `gorm:"column:question"`
	SortOrder   int       `gorm:"column:sort_order"`
	Enabled     int       `gorm:"column:enabled"`
	Deleted     int       `gorm:"column:deleted"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime"`
}

func (SampleQuestionDO) TableName() string { return "t_sample_question" }
