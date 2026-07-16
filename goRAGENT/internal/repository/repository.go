// Package repository 定义数据访问层接口，GORM/MySQL 实现见 repository/mysql。
package repository

// Repositories 聚合全部数据访问接口，由 bootstrap 装配、按需注入各 service。
type Repositories struct {
	User           UserRepository
	KnowledgeBase  KnowledgeBaseRepository
	Document       DocumentRepository
	Chunk          ChunkRepository
	IngestionTask  IngestionTaskRepository
	Conversation   ConversationRepository
	Message        MessageRepository
	Summary        SummaryRepository
	IntentNode     IntentNodeRepository
	TermMapping    TermMappingRepository
	Trace          TraceRepository
	AuditLog       AuditLogRepository
	SampleQuestion SampleQuestionRepository
	Dashboard      DashboardRepository
}
