package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/pkg/response"
	"goRAGENT/internal/service/ingestion"
	"goRAGENT/pkg/milvus"
	"gorm.io/gorm"
)

type Handler struct {
	db              *gorm.DB
	intentCache     CacheClearer
	mappingCache    CacheClearer
	milvus          *milvus.MilvusStore
	ingestionEngine *ingestion.Engine
	dataDir         string
}

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

// DB 暴露 DB 供 router 层使用
func (h *Handler) DB() *gorm.DB { return h.db }

// SetIntentCacheClearer 注入意图树缓存清除器（意图节点变更后清缓存）
func (h *Handler) SetIntentCacheClearer(c CacheClearer) *Handler {
	h.intentCache = c
	return h
}

// SetMappingCacheClearer 注入同义词映射缓存清除器（映射变更后清缓存）
func (h *Handler) SetMappingCacheClearer(c CacheClearer) *Handler {
	h.mappingCache = c
	return h
}

// SetMilvusStore 注入 Milvus Store
func (h *Handler) SetMilvusStore(m *milvus.MilvusStore) *Handler {
	h.milvus = m
	return h
}

// SetIngestionEngine 注入入库引擎
func (h *Handler) SetIngestionEngine(e *ingestion.Engine) *Handler {
	h.ingestionEngine = e
	return h
}

// SetDataDir 设置文件管理目录
func (h *Handler) SetDataDir(d string) *Handler {
	h.dataDir = d
	return h
}

// 关键词映射 CRUD（对齐 Java QueryTermMappingController）
func (h *Handler) ListMappings(c *gin.Context)  { h.listMappings(c) }
func (h *Handler) GetMapping(c *gin.Context)    { h.getMapping(c) }
func (h *Handler) CreateMapping(c *gin.Context) { h.createMapping(c) }
func (h *Handler) UpdateMapping(c *gin.Context) { h.updateMapping(c) }
func (h *Handler) DeleteMapping(c *gin.Context) { h.deleteMapping(c) }

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	d := r.Group("/dashboard")
	d.GET("/stats", h.DashboardStats)
	d.GET("/overview", h.DashboardOverview)
	d.GET("/performance", h.DashboardPerformance)
	d.GET("/trends", h.DashboardTrends)

	kb := r.Group("/knowledge-base")
	kb.GET("", h.ListKnowledgeBases)
	kb.POST("", h.CreateKnowledgeBase)
	kb.GET("/:id", h.GetKnowledgeBase)
	kb.PUT("/:id", h.UpdateKnowledgeBase)
	kb.DELETE("/:id", h.DeleteKnowledgeBase)
	kb.GET("/chunk-strategies", h.ListChunkStrategies)
	kb.GET("/:id/docs", h.ListDocuments)
	kb.POST("/:id/docs/upload", h.UploadDocument)
	kb.GET("/docs/search", h.SearchDocuments)
	kb.GET("/docs/:id", h.GetDocument)
	kb.GET("/docs/:id/preview", h.PreviewDocument)
	kb.GET("/docs/:id/file", h.DownloadDocument)
	kb.DELETE("/docs/:id", h.DeleteDocument)
	kb.PATCH("/docs/:id/enable", h.ToggleDocument)
	kb.POST("/docs/:id/chunks", h.CreateChunk)
	kb.GET("/:id/chunks", h.ListChunksByKB)
	kb.GET("/docs/:id/chunks/:chunkId", h.GetChunk)
	kb.PUT("/docs/:id/chunks/:chunkId", h.UpdateChunk)
	kb.PATCH("/docs/:id/chunks/:chunkId/enable", h.ToggleChunk)

	ds := r.Group("/datasets")
	ds.GET("", h.ListDatasets)
	ds.GET("/:id/chunks", h.ListChunks)

	ig := r.Group("/ingestion")
	ig.GET("/tasks", h.ListIngestionTasks)
	ig.GET("/tasks/:id", h.GetIngestionTask)
	ig.GET("/tasks/:id/nodes", h.GetIngestionTaskNodes)

	it := r.Group("/intent-tree")
	it.GET("", h.GetIntentTree)
	it.POST("", h.CreateIntentNode)
	it.PUT("/node/:id", h.UpdateIntentNode)
	it.DELETE("/node/:id", h.DeleteIntentNode)
	it.GET("/trees", h.GetIntentTrees)
	it.POST("/batch/enable", h.BatchEnableIntent)
	it.POST("/batch/disable", h.BatchDisableIntent)
	it.POST("/batch/delete", h.BatchDeleteIntent)

	models := r.Group("/models")
	models.GET("", h.ListModels)
	models.PUT("/:id", h.UpdateModel)

	settings := r.Group("/settings")
	settings.GET("", h.GetSettings)
	settings.PUT("", h.UpdateSettings)

	users := r.Group("/users")
	users.GET("", h.ListUsers)
	users.POST("", h.CreateUser)
	users.PUT("/:id", h.UpdateUser)
	users.DELETE("/:id", h.DeleteUser)
	users.PATCH("/:id/password", h.ChangeUserPassword)

	traces := r.Group("/traces")
	traces.GET("/runs", h.ListTraceRuns)
	traces.GET("/runs/:runId", h.GetTraceDetail)

	audit := r.Group("/biz-change-logs")
	audit.GET("", h.ListBizChangeLogs)
	audit.GET("/:id", h.GetBizChangeLog)

	sq := r.Group("/sample-questions")
	sq.GET("", h.ListSampleQuestions)
	sq.POST("", h.CreateSampleQuestion)
	sq.PUT("/:id", h.UpdateSampleQuestion)
	sq.DELETE("/:id", h.DeleteSampleQuestion)
}

// dummy handlers
func ok(c *gin.Context)                { c.JSON(200, response.Success(gin.H{})) }
func okArr(c *gin.Context)             { c.JSON(200, response.Success([]gin.H{})) }
func okID(c *gin.Context)              { c.JSON(200, response.Success(gin.H{"id": "1"})) }
func okEmpty(c *gin.Context)           { c.JSON(200, response.SuccessOK()) }

// Dashboard — real
func (h *Handler) DashboardStats(c *gin.Context)       { h.dashboardStatsReal(c) }
func (h *Handler) DashboardOverview(c *gin.Context)     { h.dashboardOverviewReal(c) }
func (h *Handler) DashboardPerformance(c *gin.Context)  { h.dashboardPerformanceReal(c) }
func (h *Handler) DashboardTrends(c *gin.Context)       { h.dashboardTrendsReal(c) }

// Knowledge base — real
func (h *Handler) ListKnowledgeBases(c *gin.Context)  { h.listKnowledgeBases(c) }
func (h *Handler) CreateKnowledgeBase(c *gin.Context) { h.createKnowledgeBase(c) }
func (h *Handler) GetKnowledgeBase(c *gin.Context)    { h.getKnowledgeBase(c) }
func (h *Handler) UpdateKnowledgeBase(c *gin.Context) { h.updateKnowledgeBase(c) }
func (h *Handler) DeleteKnowledgeBase(c *gin.Context) { h.deleteKnowledgeBase(c) }

// Documents — real
func (h *Handler) ListDocuments(c *gin.Context)      { h.listDocuments(c) }
func (h *Handler) UploadDocument(c *gin.Context)      { h.uploadDocument(c) }
func (h *Handler) SearchDocuments(c *gin.Context)     { h.searchDocuments(c) }
func (h *Handler) GetDocument(c *gin.Context)         { h.getDocument(c) }
func (h *Handler) PreviewDocument(c *gin.Context)     { h.previewDocument(c) }
func (h *Handler) DownloadDocument(c *gin.Context)    { h.downloadDocument(c) }
func (h *Handler) DeleteDocument(c *gin.Context)      { h.deleteDocument(c) }
func (h *Handler) ToggleDocument(c *gin.Context)      { h.toggleDocument(c) }

// Chunks — real
func (h *Handler) ListChunksByKB(c *gin.Context) { h.listChunksByKB(c) }
func (h *Handler) ListChunks(c *gin.Context)     { h.listChunks(c) }
func (h *Handler) GetChunk(c *gin.Context)       { h.getChunk(c) }
func (h *Handler) UpdateChunk(c *gin.Context)    { h.updateChunk(c) }
func (h *Handler) ToggleChunk(c *gin.Context)    { h.toggleChunk(c) }

// Ingestion tasks — real
func (h *Handler) ListIngestionTasks(c *gin.Context)    { h.listIngestionTasks(c) }
func (h *Handler) GetIngestionTask(c *gin.Context)      { h.getIngestionTask(c) }
func (h *Handler) GetIngestionTaskNodes(c *gin.Context) { h.getIngestionTaskNodes(c) }

// Still dummy
func (h *Handler) ListChunkStrategies(c *gin.Context) { okArr(c) }
func (h *Handler) CreateChunk(c *gin.Context)         { okID(c) }
func (h *Handler) ListDatasets(c *gin.Context)         { okArr(c) }
func (h *Handler) GetIntentTree(c *gin.Context)        { h.intentTrees(c) }
func (h *Handler) CreateIntentNode(c *gin.Context)     { h.createIntentNode(c) }
func (h *Handler) UpdateIntentNode(c *gin.Context)     { h.updateIntentNode(c) }
func (h *Handler) DeleteIntentNode(c *gin.Context)     { h.deleteIntentNode(c) }
func (h *Handler) GetIntentTrees(c *gin.Context)       { h.intentTrees(c) }
func (h *Handler) BatchEnableIntent(c *gin.Context)    { h.batchUpdateIntent(c, map[string]any{"enabled": 1}) }
func (h *Handler) BatchDisableIntent(c *gin.Context)   { h.batchUpdateIntent(c, map[string]any{"enabled": 0}) }
func (h *Handler) BatchDeleteIntent(c *gin.Context)    { h.batchUpdateIntent(c, map[string]any{"deleted": 1}) }
func (h *Handler) ListModels(c *gin.Context)           { okArr(c) }
func (h *Handler) UpdateModel(c *gin.Context)          { okEmpty(c) }
func (h *Handler) GetSettings(c *gin.Context)          { ok(c) }
func (h *Handler) UpdateSettings(c *gin.Context)       { okEmpty(c) }

// Users — real
func (h *Handler) ListUsers(c *gin.Context) { h.listUsersReal(c) }

// Trace — real
func (h *Handler) ListTraceRuns(c *gin.Context)  { h.listTraceRunsReal(c) }
func (h *Handler) GetTraceDetail(c *gin.Context) { h.getTraceDetailReal(c) }

// Audit log — real
func (h *Handler) ListBizChangeLogs(c *gin.Context) { h.listBizChangeLogs(c) }
func (h *Handler) GetBizChangeLog(c *gin.Context)   { h.getBizChangeLog(c) }

// Sample questions — real
func (h *Handler) ListSampleQuestions(c *gin.Context)      { h.listSampleQuestions(c) }
func (h *Handler) CreateSampleQuestion(c *gin.Context)     { h.createSampleQuestion(c) }
func (h *Handler) UpdateSampleQuestion(c *gin.Context)     { h.updateSampleQuestion(c) }
func (h *Handler) DeleteSampleQuestion(c *gin.Context)     { h.deleteSampleQuestion(c) }
func (h *Handler) GetSampleQuestionsPublic(c *gin.Context) { h.getSampleQuestionsPublic(c) }

// User mgmt — real
func (h *Handler) CreateUser(c *gin.Context)        { h.createUser(c) }
func (h *Handler) UpdateUser(c *gin.Context)        { h.updateUser(c) }
func (h *Handler) DeleteUser(c *gin.Context)        { h.deleteUser(c) }
func (h *Handler) ChangeUserPassword(c *gin.Context) { h.changePassword(c) }
