package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/ingestion"
	"goRAGENT/internal/rag/retrieve/vectorstore"
	"gorm.io/gorm"
)

type Handler struct {
	db              *gorm.DB
	intentCache     CacheClearer
	mappingCache    CacheClearer
	milvus          *vectorstore.MilvusStore
	ingestionEngine *ingestion.Engine
	dataDir         string
}

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

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
func (h *Handler) SetMilvusStore(m *vectorstore.MilvusStore) *Handler {
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
	kb.PATCH("/docs/:docId/enable", h.ToggleDocument)
	kb.POST("/docs/:id/chunks", h.CreateChunk)
	kb.GET("/:id/chunks", h.ListChunksByKB)
	kb.GET("/docs/:docId/chunks/:chunkId", h.GetChunk)
	kb.PUT("/docs/:docId/chunks/:chunkId", h.UpdateChunk)
	kb.PATCH("/docs/:docId/chunks/:chunkId/enable", h.ToggleChunk)

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

	traces := r.Group("/traces")
	traces.GET("/runs", h.ListTraceRuns)
	traces.GET("/runs/:runId", h.GetTraceDetail)
}

// dummy handlers
func ok(c *gin.Context)                { c.JSON(200, response.Success(gin.H{})) }
func okArr(c *gin.Context)             { c.JSON(200, response.Success([]gin.H{})) }
func okID(c *gin.Context)              { c.JSON(200, response.Success(gin.H{"id": "1"})) }
func okEmpty(c *gin.Context)           { c.JSON(200, response.SuccessOK()) }

func (h *Handler) DashboardStats(c *gin.Context) {
	c.JSON(http.StatusOK, response.Success(gin.H{
		"totalConversations": 0, "totalMessages": 0, "totalKb": 0, "totalDocuments": 0,
	}))
}
func (h *Handler) DashboardOverview(c *gin.Context) {
	kpi := func(v float64, d float64) gin.H { return gin.H{"value": v, "deltaPct": d} }
	c.JSON(http.StatusOK, response.Success(gin.H{
		"kpis": gin.H{
			"sessions24h": kpi(0, 0), "messages24h": kpi(0, 0),
			"activeUsers": kpi(0, 0), "avgLatencyMs": kpi(0, 0),
			"qualityScore": kpi(0, 0),
		},
		"topIntentNodes":     []gin.H{},
		"recentConversations": []gin.H{},
	}))
}
func (h *Handler) DashboardPerformance(c *gin.Context) {
	c.JSON(http.StatusOK, response.Success(gin.H{
		"p50Ms": 0, "p95Ms": 0, "p99Ms": 0,
		"throughput": 0, "noDocRate": 0, "avgLatencyMs": 0,
		"qualityScore": 0, "tokenTotal": 0, "tokenAvg": 0,
		"errorRate": 0, "emptyRate": 0,
	}))
}
func (h *Handler) DashboardTrends(c *gin.Context) {
	c.JSON(http.StatusOK, response.Success(gin.H{"series": []gin.H{}}))
}

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
func (h *Handler) ListUsers(c *gin.Context)            { okArr(c) }
func (h *Handler) ListTraceRuns(c *gin.Context)        { okArr(c) }
func (h *Handler) GetTraceDetail(c *gin.Context)       { ok(c) }
