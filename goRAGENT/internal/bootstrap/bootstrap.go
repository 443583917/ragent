package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"goRAGENT/internal/config"
	"goRAGENT/internal/handler/admin"
	"goRAGENT/internal/handler/chat"
	"goRAGENT/internal/handler/session"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
	mysqlrepo "goRAGENT/internal/repository/mysql"
	"goRAGENT/internal/router"
	svcadmin "goRAGENT/internal/service/admin"
	authsvc "goRAGENT/internal/service/auth"
	"goRAGENT/internal/service/ingestion"
	"goRAGENT/internal/service/mcp"
	"goRAGENT/internal/service/rag"
	"goRAGENT/pkg/embedding"
	"goRAGENT/pkg/llm"
	"goRAGENT/pkg/milvus"
	"goRAGENT/pkg/mineru"
	"goRAGENT/pkg/prompt"
	"goRAGENT/pkg/rerank"
)

// App 持有全部装配后的依赖，提供 Run() 启动 HTTP 服务。
type App struct {
	cfg            *config.Config
	db             *gorm.DB
	rdb            *redis.Client
	adminH         *admin.Handler
	chatHandler    *chat.ChatHandler
	chatLimiter    *middleware.Limiter
	authSvc        authsvc.AuthService
	sessionHandler *session.Handler
}

// New 装配全部依赖并返回 App。
// db 为 nil 时降级启动（无 DB 的服务在 handler 层会返回 500），不会阻塞启动。
func New(cfg *config.Config) (*App, error) {
	db, err := initDB(cfg)
	if err != nil {
		zap.L().Warn("数据库连接失败（非致命），以无 DB 模式启动", zap.Error(err))
	}
	rdb := initRedis(cfg)

	probeDB(db)
	probeRedis(rdb)
	probeMilvus(cfg.Milvus.URI())
	go probeEmbedding(cfg.Embedding.HTTPURL)
	pm := cfg.LLM.Resolve(cfg.LLM.PrimaryProvider())
	go probeLLM(cfg.LLM.PrimaryProvider(), pm.Model, pm.BaseURL, pm.Key)

	// ====== 依赖装配 ======
	repos := mysqlrepo.New(db)
	authSvc := authsvc.NewAuthService(repos.User, authsvc.NewMD5PasswordHasher())
	llmSvc := llm.NewChatService(cfg)
	prompts := prompt.NewTemplateLoader()
	memSvc := rag.NewConversationMemory(cfg, repos.Conversation, repos.Message, repos.Summary, rdb, llmSvc, prompts)
	embedSvc := embedding.NewService(cfg.Embedding.HTTPURL)
	rerankSvc := rerank.NewService(cfg.Reranker.HTTPURL, cfg.RAG.RerankTopK)
	mineruClient := mineru.NewClient(cfg.Mineru.APIToken)

	var mvStore *milvus.MilvusStore
	var ingestionEngine *ingestion.Engine
	var searchChannels []model.SearchChannel
	if mvStore, err = milvus.NewMilvusStore(cfg.Milvus.URI(), embedSvc); err == nil {
		searchChannels = append(searchChannels,
			rag.NewIntentDirectedChannel(cfg.RAG.Search.Channels.IntentDirected, mvStore),
			rag.NewVectorGlobalChannel(cfg.RAG.Search.Channels.VectorGlobal, true, mvStore),
		)
		ingestionEngine = ingestion.NewEngine(repos.IngestionTask, repos.Document, repos.KnowledgeBase, repos.Chunk, cfg.Mineru.DataDir, mineruClient, embedSvc, mvStore, cfg.Ingestion)
		if cfg.RAG.Search.Channels.WebSearch.Enabled && cfg.RAG.Search.Channels.WebSearch.APIKey != "" {
			searchChannels = append(searchChannels,
				rag.NewYouComWebSearchChannel(cfg.RAG.Search.Channels.WebSearch),
			)
		}
	}
	postProcessors := []rag.PostProcessor{
		rag.NewMetadataEnrichmentPostProcessor(repos.Chunk, repos.Document),
		&rag.DedupPostProcessor{},
		&rag.FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 50},
		rag.NewRerankPostProcessor(rag.RerankerAdapter(rerankSvc), cfg.RAG.RerankEnabled),
	}
	multiChannelEngine := rag.NewMultiChannelEngine(searchChannels, postProcessors)
	retrievalEngine := rag.NewRetrievalEngine(cfg.RAG, multiChannelEngine, prompts)

	intentLoader := rag.NewTreeLoader(repos.IntentNode, rdb)
	intentClassifier := rag.NewClassifier(intentLoader, llmSvc, prompts)
	intentResolver := rag.NewResolver(intentClassifier)
	mappingLoader := rag.NewMappingLoader(repos.TermMapping, rdb)
	queryRewriter := rag.NewRewriter(mappingLoader, llmSvc, prompts, cfg.RAG.QueryRewrite)
	guidanceDetector := rag.NewDetector(cfg.Guidance, llmSvc, prompts)

	ragPipeline := rag.NewSimplePipeline(cfg, memSvc, llmSvc, prompts, retrievalEngine, queryRewriter, intentResolver, guidanceDetector)

	if len(cfg.Mcp.Servers) > 0 {
		mcpRegistry := mcp.NewRegistry(cfg.Mcp.Servers)
		mcpExtractor := mcp.NewExtractor(llmSvc, prompts)
		mcpFormatter := mcp.NewFormatter()
		mcpExecutor := mcp.NewExecutor(mcpRegistry, mcpExtractor, mcpFormatter)
		ragPipeline.SetMcpExecutor(mcpExecutor, mcpFormatter)
		zap.L().Info("MCP 已启用", zap.Int("servers", len(cfg.Mcp.Servers)))
	}

	// 会话业务服务 + HTTP handler
	sessionSvc := rag.NewSessionService(repos.Conversation, repos.Message)
	sessionHandler := session.NewHandler(sessionSvc)

	// 追踪记录器 + chat handler
	traceRecorder := rag.NewTraceRecorder(repos.Trace)
	chatHandler := chat.NewSimpleChatHandler(cfg, ragPipeline, traceRecorder)

	services := svcadmin.NewServices(
		&repos,
		mvStore,
		intentLoader,
		mappingLoader,
		ingestionEngine,
		authsvc.NewMD5PasswordHasher(),
		cfg.Mineru.DataDir,
	)
	adminH := admin.NewHandler(*services)

	var chatLimiter *middleware.Limiter
	if rdb != nil {
		chatLimiter = middleware.NewLimiter(rdb, middleware.Config{
			Enabled: true, MaxConcurrent: 10, MaxWaitSeconds: 15,
			LeaseSeconds: 30, PollIntervalMs: 200,
		})
	}

	app := &App{
		cfg:            cfg, db: db, rdb: rdb,
		authSvc:        authSvc,
		adminH:         adminH,
		chatHandler:    chatHandler,
		chatLimiter:    chatLimiter,
		sessionHandler: sessionHandler,
	}
	return app, nil
}

// Run 启动 gin HTTP 服务，包含优雅关闭。
func (a *App) Run() {
	gin.SetMode(map[bool]string{true: gin.DebugMode, false: gin.ReleaseMode}[a.cfg.App.Debug])
	r := gin.New()
	r.Use(gin.Recovery())
	router.Register(r, router.Deps{
		Cfg: a.cfg, DB: a.db, RDB: a.rdb,
		AdminH: a.adminH, ChatHandler: a.chatHandler, ChatLimiter: a.chatLimiter,
		AuthSvc: a.authSvc, SessionHandler: a.sessionHandler,
	})

	addr := fmt.Sprintf("%s:%d", a.cfg.App.Host, a.cfg.App.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		zap.L().Info("关闭服务...")
		_ = srv.Shutdown(context.Background())
	}()

	zap.L().Info("服务启动", zap.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("启动失败: %v", err)
	}
}

// initDB 数据库连接。
// 连接失败返回 nil, error；调用方记 Warn 后可继续降级启动。
func initDB(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		zap.L().Warn("获取 DB 连接池失败", zap.Error(err))
		return db, nil
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.PoolSize)
	sqlDB.SetMaxIdleConns(cfg.MySQL.PoolSize / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// initRedis 创建 Redis 客户端。
func initRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Host + ":" + fmt.Sprintf("%d", cfg.Redis.Port),
		Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
}

// PrintStartupBanner 打印启动横幅。
func PrintStartupBanner(appName, llmProvider string) {
	zap.L().Info("══════════════════════════════════════════")
	zap.L().Info(fmt.Sprintf("  %s v1.0.0 启动中...", appName))
	zap.L().Info(fmt.Sprintf("  LLM Provider: %s", llmProvider))
	zap.L().Info("══════════════════════════════════════════")
}
