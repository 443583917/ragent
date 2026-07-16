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

	"goRAGENT/internal/config"
	"goRAGENT/internal/handler/admin"
	"goRAGENT/internal/handler/chat"
	"goRAGENT/internal/middleware"
	"goRAGENT/internal/model"
	"goRAGENT/internal/router"
	"goRAGENT/internal/service/ingestion"
	"goRAGENT/internal/service/mcp"
	"goRAGENT/internal/service/rag"
	"goRAGENT/pkg/embedding"
	"goRAGENT/pkg/llm"
	"goRAGENT/pkg/logx"
	"goRAGENT/pkg/milvus"
	"goRAGENT/pkg/mineru"
	"goRAGENT/pkg/prompt"
	"goRAGENT/pkg/rerank"
	"goRAGENT/pkg/snowflake"
)

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadDotEnv(".env")
	cfg := config.Load()

	logger := logx.Init(strings.ToLower(cfg.Log.Level))
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	if err := snowflake.Init(1); err != nil {
		log.Fatalf("Snowflake: %v", err)
	}

	db, _ := initDB(cfg)
	if db != nil {
		config.SetDBGorm(db)
	}
	rdb := initRedis(cfg)
	if rdb != nil {
		config.SetRedisClient(rdb)
	}

	PrintStartupBanner(cfg.App.Name, cfg.LLM.PrimaryProvider())
	InitDB(db)
	InitRedis(rdb)
	InitMilvus(cfg.Milvus.URI())
	go InitEmbedding(cfg.Embedding.HTTPURL)
	go InitLLM(cfg.LLM.PrimaryProvider(), "", "", "")

	// ====== 依赖装配 ======
	llmSvc := llm.NewChatService(cfg)
	prompts := prompt.NewTemplateLoader()
	memSvc := rag.NewConversationMemory(cfg, db, rdb, llmSvc, prompts)
	embedSvc := embedding.NewService(cfg.Embedding.HTTPURL)
	rerankSvc := rerank.NewService(cfg.Reranker.HTTPURL, cfg.RAG.RerankTopK)
	mineruClient := mineru.NewClient(cfg.Mineru.APIToken)

	var mvStore *milvus.MilvusStore
	var ingestionEngine *ingestion.Engine
	var searchChannels []model.SearchChannel
	var err error
	if mvStore, err = milvus.NewMilvusStore(cfg.Milvus.URI(), embedSvc); err == nil {
		searchChannels = append(searchChannels,
			rag.NewIntentDirectedChannel(cfg.RAG.Search.Channels.IntentDirected, mvStore),
			rag.NewVectorGlobalChannel(cfg.RAG.Search.Channels.VectorGlobal, true, mvStore),
		)
		ingestionEngine = ingestion.NewEngine(db, mineruClient, embedSvc, mvStore, cfg.Ingestion)
		if cfg.RAG.Search.Channels.WebSearch.Enabled && cfg.RAG.Search.Channels.WebSearch.APIKey != "" {
			searchChannels = append(searchChannels,
				rag.NewYouComWebSearchChannel(cfg.RAG.Search.Channels.WebSearch),
			)
		}
	}
	postProcessors := []rag.PostProcessor{
		rag.NewMetadataEnrichmentPostProcessor(db),
		&rag.DedupPostProcessor{},
		&rag.FusionPostProcessor{RRFK: 60, RerankCandidateLimit: 50},
		rag.NewRerankPostProcessor(rag.RerankerAdapter(rerankSvc), cfg.RAG.RerankEnabled),
	}
	multiChannelEngine := rag.NewMultiChannelEngine(searchChannels, postProcessors)
	retrievalEngine := rag.NewRetrievalEngine(cfg.RAG, multiChannelEngine, prompts)

	intentLoader := rag.NewTreeLoader(db, rdb)
	intentClassifier := rag.NewClassifier(intentLoader, llmSvc, prompts)
	intentResolver := rag.NewResolver(intentClassifier)
	mappingLoader := rag.NewMappingLoader(db, rdb)
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

	chatHandler := chat.NewSimpleChatHandler(cfg, ragPipeline, db)
	adminH := admin.NewHandler(db).
		SetIntentCacheClearer(intentLoader).
		SetMappingCacheClearer(mappingLoader).
		SetMilvusStore(mvStore).
		SetIngestionEngine(ingestionEngine).
		SetDataDir(cfg.Mineru.DataDir)

	var chatLimiter *middleware.Limiter
	if rdb != nil {
		chatLimiter = middleware.NewLimiter(rdb, middleware.Config{
			Enabled: true, MaxConcurrent: 10, MaxWaitSeconds: 15,
			LeaseSeconds: 30, PollIntervalMs: 200,
		})
	}

	// ====== 路由 + 启动 ======
	gin.SetMode(map[bool]string{true: gin.DebugMode, false: gin.ReleaseMode}[cfg.App.Debug])
	r := gin.New()
	r.Use(gin.Recovery())
	router.Register(r, router.Deps{
		Cfg: cfg, AdminH: adminH, ChatHandler: chatHandler, ChatLimiter: chatLimiter,
	})

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
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
	if err != nil {
		return nil, err
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.MySQL.PoolSize)
	sqlDB.SetMaxIdleConns(cfg.MySQL.PoolSize / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func initRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Host + ":" + fmt.Sprintf("%d", cfg.Redis.Port),
		Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
}
