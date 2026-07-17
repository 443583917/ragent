package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"goRAGENT/internal/config"
	"goRAGENT/internal/handler/admin"
	"goRAGENT/internal/handler/auth"
	"goRAGENT/internal/handler/chat"
	"goRAGENT/internal/middleware"
	authsvc "goRAGENT/internal/service/auth"
	"goRAGENT/internal/service/rag"
	"goRAGENT/pkg/jwt"
)

// Deps 路由层依赖（由 main 装配后传入）
type Deps struct {
	Cfg         *config.Config
	AdminH      *admin.Handler
	ChatHandler *chat.ChatHandler
	ChatLimiter *middleware.Limiter
	AuthSvc     authsvc.AuthService
	SessionDB   *gorm.DB // TODO(Task 6): 移除，session handler 改为 service 注入
}

// Register 注册全部路由到 gin.Engine
func Register(r *gin.Engine, d Deps) {
	api := r.Group("/api/ragent")
	api.GET("/health", HealthHandler(d.Cfg))

	// 认证（无需 JWT）
	authH := auth.NewHandler(d.AuthSvc)
	authH.AuthRoutes(api.Group("/auth"))
	api.POST("/auth/logout", func(c *gin.Context) { c.JSON(200, gin.H{"code": "0"}) })

	// 示例问题（无需 JWT）
	api.GET("/rag/sample-questions", d.AdminH.GetSampleQuestionsPublic)

	// RAG 对话（JWT + 限流）
	ragV3 := api.Group("/rag/v3")
	ragV3.Use(jwt.Middleware(d.Cfg.SaToken.TokenName))
	ragV3.GET("/chat", d.chatRoute())
	ragV3.POST("/stop", d.ChatHandler.StopTask)

	// 会话 + 消息 + 反馈（JWT）
	sessH := rag.NewSessionHandler(d.SessionDB, d.Cfg)
	sessionGroup := api.Group("", jwt.Middleware(d.Cfg.SaToken.TokenName))
	sessH.RegisterRoutes(sessionGroup)

	// 用户信息（JWT）
	api.GET("/user/me", jwt.Middleware(d.Cfg.SaToken.TokenName), authH.CurrentUser)

	// 前台管理接口
	registerFrontendRoutes(api, d.AdminH)

	// 管理后台
	registerAdminRoutes(api, d.AdminH, d.Cfg)

	// 管理后台子路由（/admin 前缀）
	d.AdminH.RegisterRoutes(api.Group("/admin"))
}

func (d Deps) chatRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.ChatLimiter != nil {
			taskID := c.Query("taskId")
			pos, _, ok := d.ChatLimiter.TryAcquire(taskID)
			if !ok {
				c.JSON(429, gin.H{"code": "C000001", "message": "系统繁忙"})
				return
			}
			_ = pos
		}
		d.ChatHandler.StreamChat(c)
	}
}

func registerFrontendRoutes(api *gin.RouterGroup, h *admin.Handler) {
	api.GET("/knowledge-base", h.ListKnowledgeBases)
	api.POST("/knowledge-base", h.CreateKnowledgeBase)
	api.GET("/knowledge-base/:id", h.GetKnowledgeBase)
	api.PUT("/knowledge-base/:id", h.UpdateKnowledgeBase)
	api.DELETE("/knowledge-base/:id", h.DeleteKnowledgeBase)
	api.GET("/knowledge-base/docs/search", h.SearchDocuments)
	api.GET("/intent-tree", h.GetIntentTree)
	api.POST("/intent-tree", h.CreateIntentNode)
	api.GET("/intent-tree/trees", h.GetIntentTrees)
	api.POST("/intent-tree/batch/enable", h.BatchEnableIntent)
	api.POST("/intent-tree/batch/disable", h.BatchDisableIntent)
	api.POST("/intent-tree/batch/delete", h.BatchDeleteIntent)
	api.GET("/models", h.ListModels)
	api.GET("/settings", h.GetSettings)
	api.PUT("/settings", h.UpdateSettings)
	api.GET("/traces/runs", h.ListTraceRuns)
	api.GET("/traces/runs/:runId", h.GetTraceDetail)
	api.GET("/knowledge-base/docs/:id/chunks/:chunkId", h.GetChunk)
	api.PUT("/knowledge-base/docs/:id/chunks/:chunkId", h.UpdateChunk)
	api.PATCH("/knowledge-base/docs/:id/chunks/:chunkId/enable", h.ToggleChunk)
	api.PATCH("/knowledge-base/docs/:id/enable", h.ToggleDocument)
	api.PUT("/intent-tree/:id", h.UpdateIntentNode)
	api.DELETE("/intent-tree/:id", h.DeleteIntentNode)
}

func registerAdminRoutes(api *gin.RouterGroup, h *admin.Handler, cfg *config.Config) {
	api.GET("/rag/settings", SettingsHandler(cfg))
	api.PUT("/rag/settings", func(c *gin.Context) { c.JSON(200, gin.H{"code": "0"}) })

	api.GET("/rag/traces/runs", h.ListTraceRuns)
	api.GET("/rag/traces/runs/:traceId", h.GetTraceDetail)
	api.GET("/rag/traces/runs/:traceId/nodes", h.GetTraceNodes)

	api.GET("/sample-questions", h.ListSampleQuestions)
	api.POST("/sample-questions", h.CreateSampleQuestion)
	api.PUT("/sample-questions/:id", h.UpdateSampleQuestion)
	api.DELETE("/sample-questions/:id", h.DeleteSampleQuestion)

	api.GET("/mappings", h.ListMappings)
	api.GET("/mappings/:id", h.GetMapping)
	api.POST("/mappings", h.CreateMapping)
	api.PUT("/mappings/:id", h.UpdateMapping)
	api.DELETE("/mappings/:id", h.DeleteMapping)

	api.GET("/biz-change-logs", h.ListBizChangeLogs)
	api.GET("/biz-change-logs/:id", h.GetBizChangeLog)

	api.GET("/users", h.ListUsers)
	api.POST("/users", h.CreateUser)
	api.PUT("/users/:id", h.UpdateUser)
	api.DELETE("/users/:id", h.DeleteUser)
	api.PATCH("/users/:id/password", h.ChangeUserPassword)

	api.GET("/ingestion/tasks", h.ListIngestionTasks)
	api.GET("/ingestion/tasks/:id", h.GetIngestionTask)
	api.GET("/ingestion/tasks/:id/nodes", h.GetIngestionTaskNodes)

	// 入库管线（预留）
	api.GET("/ingestion/pipelines", func(c *gin.Context) { c.JSON(200, gin.H{"code": "0", "data": gin.H{"total": 0, "rows": []gin.H{}}}) })
	api.GET("/ingestion/pipelines/:id", func(c *gin.Context) { c.JSON(200, gin.H{"code": "0", "data": gin.H{}}) })
}
