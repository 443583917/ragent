// Package mysql 提供 repository 接口的 GORM/MySQL 实现。
package mysql

import (
	"gorm.io/gorm"

	"goRAGENT/internal/repository"
)

// New 装配全部 MySQL repository 实现。
func New(db *gorm.DB) repository.Repositories {
	return repository.Repositories{
		User:           NewUserRepo(db),
		KnowledgeBase:  NewKnowledgeBaseRepo(db),
		Document:       NewDocumentRepo(db),
		Chunk:          NewChunkRepo(db),
		IngestionTask:  NewIngestionTaskRepo(db),
		Conversation:   NewConversationRepo(db),
		Message:        NewMessageRepo(db),
		Summary:        NewSummaryRepo(db),
		IntentNode:     NewIntentNodeRepo(db),
		TermMapping:    NewTermMappingRepo(db),
		Trace:          NewTraceRepo(db),
		AuditLog:       NewAuditLogRepo(db),
		SampleQuestion: NewSampleQuestionRepo(db),
		Dashboard:      NewDashboardRepo(db),
	}
}

// notDeleted 软删除过滤公共 Scope。
func notDeleted(db *gorm.DB) *gorm.DB { return db.Where("deleted = 0") }
