// Package admin 提供管理后台域服务。
//
// 包含 11 个域的服务接口与实现：Dashboard / KnowledgeBase / Document / Chunk /
// IngestionTask / Intent / Mapping / User / Trace / Audit / SampleQuestion。
// 所有 service 不依赖 gin/gorm，仅依赖 repository 接口 + model 类型 +
// pkg/errs + pkg/snowflake + go.uber.org/zap。
package admin

import (
	"context"

	"goRAGENT/internal/repository"
	"goRAGENT/internal/service/auth"
)

// CacheClearer 意图树/同义词映射缓存清除抽象（*model.TreeLoader 满足此接口）。
type CacheClearer interface {
	ClearCache(ctx context.Context)
}

// Services 聚合全部管理后台域服务，由 bootstrap 装配并按需注入各 handler。
type Services struct {
	Dashboard          DashboardService
	KnowledgeBase      KnowledgeBaseService
	Document           DocumentService
	Chunk              ChunkService
	IngestionTask      IngestionTaskService
	Intent             IntentService
	Mapping            MappingService
	User               UserService
	Trace              TraceService
	Audit              AuditService
	SampleQuestion     SampleQuestionService
}

// NewServices 构造管理后台所有域服务。
//
// 依赖注入说明：
//   - repos: 全部 14 个 repository 实现（从 mysqlrepo.New(db) 获取）
//   - vectorStore: Milvus 向量库抽象，可为 nil
//   - intentCacheClearer: 意图树缓存清除器（*model.TreeLoader），可为 nil
//   - mappingCacheClearer: 同义词映射缓存清除器（*model.TreeLoader），可为 nil
//   - ingestor: 入库引擎（*ingestion.Engine），可为 nil
//   - hasher: 密码哈希器（auth.MD5PasswordHasher）
//   - dataDir: 文件管理根目录
func NewServices(
	repos *repository.Repositories,
	vectorStore VectorStore,
	intentCacheClearer CacheClearer,
	mappingCacheClearer CacheClearer,
	ingestor Ingestor,
	hasher auth.PasswordHasher,
	dataDir string,
) *Services {
	return &Services{
		Dashboard:      NewDashboardService(repos.Dashboard),
		KnowledgeBase:  NewKnowledgeBaseService(repos.KnowledgeBase, vectorStore),
		Document:       NewDocumentService(repos.Document, repos.Chunk, repos.IngestionTask, repos.KnowledgeBase, ingestor, dataDir),
		Chunk:          NewChunkService(repos.Chunk),
		IngestionTask:  NewIngestionTaskService(repos.IngestionTask),
		Intent:         NewIntentService(repos.IntentNode, intentCacheClearer),
		Mapping:        NewMappingService(repos.TermMapping, mappingCacheClearer),
		User:           NewUserService(repos.User, hasher),
		Trace:          NewTraceService(repos.Trace),
		Audit:          NewAuditService(repos.AuditLog),
		SampleQuestion: NewSampleQuestionService(repos.SampleQuestion),
	}
}
