package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"goRAGENT/internal/admin"
	"goRAGENT/internal/config"
	"goRAGENT/internal/framework/jwt"
	"goRAGENT/internal/framework/logx"
	"goRAGENT/internal/framework/snowflake"
	"goRAGENT/internal/infra/embedding"
	"goRAGENT/internal/infra/llm"
	"goRAGENT/internal/framework/ratelimit"
	"goRAGENT/internal/infra/mineru"
	"goRAGENT/internal/infra/rerank"
	"goRAGENT/internal/ingestion"
	"goRAGENT/internal/rag"
	"goRAGENT/internal/rag/guidance"
	"goRAGENT/internal/rag/intent"
	"goRAGENT/internal/rag/mcp"
	"goRAGENT/internal/rag/memory"
	"goRAGENT/internal/rag/pipeline"
	"goRAGENT/internal/rag/prompt"
	"goRAGENT/internal/rag/retrieve"
	"goRAGENT/internal/rag/retrieve/postprocessor"
	"goRAGENT/internal/rag/retrieve/vectorstore"
	"goRAGENT/internal/rag/rewrite"
	"goRAGENT/internal/user"
)

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil { return }
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") { continue }
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 { continue }
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		if os.Getenv(k) == "" { os.Setenv(k, v) }
	}
}

func main() {
	loadDotEnv(".env")
	cfg := config.Load()

	logger := logx.Init(strings.ToLower(cfg.Log.Level))
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	if err := snowflake.Init(1); err != nil { log.Fatalf("Snowflake: %v", err) }

	db, _ := initDB(cfg)
	if db != nil { config.SetDBGorm(db) }

	rdb := initRedis(cfg)
	if rdb != nil { config.SetRedisClient(rdb) }

	PrintStartupBanner(cfg.App.Name, cfg.LLM.PrimaryProvider())
	InitDB(db)
	InitRedis(rdb)
	InitMilvus(cfg.Milvus.URI())
	go InitEmbedding(cfg.Embedding.HTTPURL)
	go InitLLM(cfg.LLM.PrimaryProvider(), "", "", "")

	llmSvc := llm.NewChatService(cfg)
	prompts := prompt.NewTemplateLoader()
	memSvc := memory.NewConversationMemory(cfg, db, rdb, llmSvc, prompts)

	embedSvc := embedding.NewService(cfg.Embedding.HTTPURL)
	rerankSvc := rerank.NewService(cfg.Reranker.HTTPURL, cfg.RAG.RerankTopK)

	// M4: MinerU + 入库引擎
	mineruClient := mineru.NewClient(cfg.Mineru.APIToken)
	var mvStore *vectorstore.MilvusStore
	var ingestionEngine *ingestion.Engine

	var searchChannels []retrieve.SearchChannel
	var err error
	if mvStore, err = vectorstore.NewMilvusStore(cfg.Milvus.URI(), embedSvc); err == nil {
		searchChannels = append(searchChannels,
			retrieve.NewIntentDirectedChannel(cfg.RAG.Search.Channels.IntentDirected, mvStore),
			retrieve.NewVectorGlobalChannel(cfg.RAG.Search.Channels.VectorGlobal, true, mvStore),
		)
		ingestionEngine = ingestion.NewEngine(db, mineruClient, embedSvc, mvStore, cfg.Ingestion)

		// M5: You.com 联网检索（最低优先级兜底）
		if cfg.RAG.Search.Channels.WebSearch.Enabled && cfg.RAG.Search.Channels.WebSearch.APIKey != "" {
			searchChannels = append(searchChannels,
				retrieve.NewYouComWebSearchChannel(cfg.RAG.Search.Channels.WebSearch),
			)
		}
	}
	postProcessors := []retrieve.PostProcessor{
		postprocessor.NewMetadataEnrichmentPostProcessor(db),
		&retrieve.DedupPostProcessor{},
		&retrieve.FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 50},
		retrieve.NewRerankPostProcessor(retrieve.RerankerAdapter(rerankSvc), cfg.RAG.RerankEnabled),
	}
	multiChannelEngine := retrieve.NewMultiChannelEngine(searchChannels, postProcessors)
	retrievalEngine := retrieve.NewRetrievalEngine(cfg.RAG, multiChannelEngine, prompts)

	// 意图树: Loader(Redis→MySQL) → Classifier(LLM) → Resolver
	intentLoader := intent.NewTreeLoader(db, rdb)
	intentClassifier := intent.NewClassifier(intentLoader, llmSvc, prompts)
	intentResolver := intent.NewResolver(intentClassifier)

	// 查询改写: 同义词归一化(Redis→MySQL) → LLM 改写+子问题拆分
	mappingLoader := rewrite.NewMappingLoader(db, rdb)
	queryRewriter := rewrite.NewRewriter(mappingLoader, llmSvc, prompts, cfg.RAG.QueryRewrite)

	// 歧义引导: 分数比值 + LLM 二次确认 → 选项话术短路
	guidanceDetector := guidance.NewDetector(cfg.Guidance, llmSvc, prompts)

	ragPipeline := pipeline.NewSimplePipeline(cfg, memSvc, llmSvc, prompts, retrievalEngine, queryRewriter, intentResolver, guidanceDetector)

	// M5: MCP 工具执行器
	if len(cfg.Mcp.Servers) > 0 {
		mcpRegistry := mcp.NewRegistry(cfg.Mcp.Servers)
		mcpExtractor := mcp.NewExtractor(llmSvc, prompts)
		mcpFormatter := mcp.NewFormatter()
		mcpExecutor := mcp.NewExecutor(mcpRegistry, mcpExtractor, mcpFormatter)
		ragPipeline.SetMcpExecutor(mcpExecutor, mcpFormatter)
		zap.L().Info("MCP 已启用", zap.Int("servers", len(cfg.Mcp.Servers)))
	}

	chatHandler := pipeline.NewSimpleChatHandler(cfg, ragPipeline, db)

	// 管理后台共享 handler（意图/映射变更后清 Redis 缓存）
	adminH := admin.NewHandler(db).
		SetIntentCacheClearer(intentLoader).
		SetMappingCacheClearer(mappingLoader).
		SetMilvusStore(mvStore).
		SetIngestionEngine(ingestionEngine).
		SetDataDir(cfg.Mineru.DataDir)

	// M6: 分布式限流
	var chatLimiter *ratelimit.Limiter
	if rdb != nil {
		chatLimiter = ratelimit.NewLimiter(rdb, ratelimit.Config{
			Enabled:        true,
			MaxConcurrent:  10,
			MaxWaitSeconds: 15,
			LeaseSeconds:   30,
			PollIntervalMs: 200,
		})
	}

	// ====== 路由 ======
	gin.SetMode(map[bool]string{true: gin.DebugMode, false: gin.ReleaseMode}[cfg.App.Debug])
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api/ragent")
	api.GET("/health", HealthHandler(cfg))

	// 认证（无需 JWT）
	authH := user.NewHandler(db, cfg)
	authH.AuthRoutes(api.Group("/auth"))
api.POST("/auth/logout", func(c *gin.Context) { c.JSON(200, gin.H{"code":"0"}) })

	// 示例问题（无需 JWT）
	api.GET("/rag/sample-questions", adminH.GetSampleQuestionsPublic)

	// RAG 对话（JWT + 限流）
	ragV3 := api.Group("/rag/v3")
	ragV3.Use(jwt.Middleware(cfg.SaToken.TokenName))
	ragV3.GET("/chat", func(c *gin.Context) {
		if chatLimiter != nil {
			taskID := c.Query("taskId") // will be generated in handler
			pos, _, ok := chatLimiter.TryAcquire(taskID)
			if !ok {
				c.JSON(429, gin.H{"code": "C000001", "message": "系统繁忙"})
				return
			}
			if pos > 0 {
				_ = pos // position-based queuing handled in handler via WaitForSlot
			}
		}
		chatHandler.StreamChat(c)
	})
	ragV3.POST("/stop", chatHandler.StopTask)

	// 会话 + 消息 + 反馈（JWT，路径契约和前端 sessionService/chatService 一致）
	sessH := rag.NewSessionHandler(db, cfg)
	sessionGroup := api.Group("", jwt.Middleware(cfg.SaToken.TokenName))
	sessH.RegisterRoutes(sessionGroup)

	// 用户信息（JWT）
	api.GET("/user/me", jwt.Middleware(cfg.SaToken.TokenName), user.CurrentUser(db, cfg))

	// 前台管理接口（不带 /admin 前缀, 避免路由冲突, 仅加知识库/意图树/模型/设置/trace）
	api.GET("/knowledge-base", adminH.ListKnowledgeBases)
	api.POST("/knowledge-base", adminH.CreateKnowledgeBase)
	api.GET("/knowledge-base/:id", adminH.GetKnowledgeBase)
	api.PUT("/knowledge-base/:id", adminH.UpdateKnowledgeBase)
	api.DELETE("/knowledge-base/:id", adminH.DeleteKnowledgeBase)
	api.GET("/knowledge-base/docs/search", adminH.SearchDocuments)
	api.GET("/intent-tree", adminH.GetIntentTree)
	api.POST("/intent-tree", adminH.CreateIntentNode)
	api.GET("/intent-tree/trees", adminH.GetIntentTrees)
	api.POST("/intent-tree/batch/enable", adminH.BatchEnableIntent)
	api.POST("/intent-tree/batch/disable", adminH.BatchDisableIntent)
	api.POST("/intent-tree/batch/delete", adminH.BatchDeleteIntent)
	api.GET("/models", admin.NewHandler(db).ListModels)
	api.GET("/settings", admin.NewHandler(db).GetSettings)
	api.PUT("/settings", admin.NewHandler(db).UpdateSettings)
	api.GET("/traces/runs", admin.NewHandler(db).ListTraceRuns)
	api.GET("/traces/runs/:runId", admin.NewHandler(db).GetTraceDetail)

	// 管理后台（JWT）
	// 系统设置
	api.GET("/rag/settings", SettingsHandler(cfg))
	api.PUT("/rag/settings", func(c *gin.Context) { c.JSON(200, gin.H{"code":"0"}) })
	// Trace
	api.GET("/rag/traces/runs", adminH.ListTraceRuns)
	api.GET("/rag/traces/runs/:traceId", adminH.GetTraceDetail)
	api.GET("/rag/traces/runs/:traceId/nodes", adminH.GetTraceNodes)
	// 示例问题 CRUD
	api.GET("/sample-questions", adminH.ListSampleQuestions)
	api.POST("/sample-questions", adminH.CreateSampleQuestion)
	// 关键词映射
	api.GET("/mappings", adminH.ListMappings)
	api.GET("/mappings/:id", adminH.GetMapping)
	api.POST("/mappings", adminH.CreateMapping)
	// 审计日志
	api.GET("/biz-change-logs", adminH.ListBizChangeLogs)
	api.GET("/biz-change-logs/:id", adminH.GetBizChangeLog)
	// 用户管理
	api.GET("/users", adminH.ListUsers)
	api.POST("/users", adminH.CreateUser)
	// 入库
	api.GET("/ingestion/pipelines", func(c *gin.Context) { c.JSON(200, gin.H{"code":"0","data":gin.H{"total":0,"rows":[]gin.H{}}}) })
	api.GET("/ingestion/pipelines/:id", func(c *gin.Context) { c.JSON(200, gin.H{"code":"0","data":gin.H{}}) })
	api.GET("/ingestion/tasks", adminH.ListIngestionTasks)
	api.GET("/ingestion/tasks/:id", adminH.GetIngestionTask)
	api.GET("/ingestion/tasks/:id/nodes", adminH.GetIngestionTaskNodes)

	// 前台也用的管理接口（不带 /admin 前缀）
	api.GET("/knowledge-base/docs/:docId/chunks/:chunkId", adminH.GetChunk)
	api.PUT("/knowledge-base/docs/:docId/chunks/:chunkId", adminH.UpdateChunk)
	api.PATCH("/knowledge-base/docs/:docId/chunks/:chunkId/enable", adminH.ToggleChunk)
	api.PATCH("/knowledge-base/docs/:docId/enable", adminH.ToggleDocument)
	api.PUT("/mappings/:id", adminH.UpdateMapping)
	api.DELETE("/mappings/:id", adminH.DeleteMapping)
	api.PUT("/sample-questions/:id", adminH.UpdateSampleQuestion)
	api.DELETE("/sample-questions/:id", adminH.DeleteSampleQuestion)
	api.PUT("/users/:id", adminH.UpdateUser)
	api.DELETE("/users/:id", adminH.DeleteUser)
	api.PATCH("/users/:id/password", adminH.ChangeUserPassword)
	api.PUT("/intent-tree/:id", adminH.UpdateIntentNode)
	api.DELETE("/intent-tree/:id", adminH.DeleteIntentNode)


	adminH.RegisterRoutes(api.Group("/admin"))

	// ====== 启动 ======
	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		zap.L().Info("关闭服务...")
		srv.Shutdown(context.Background())
	}()

	zap.L().Info("服务启动", zap.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("启动失败: %v", err)
	}
}

func initDB(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil { return nil, err }
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.MySQL.PoolSize)
	sqlDB.SetMaxIdleConns(cfg.MySQL.PoolSize / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func initRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
}

func init() {
	// register additional routes in serve
}
