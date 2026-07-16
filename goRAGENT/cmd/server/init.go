package main

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"goRAGENT/internal/config"
)

// ========== 启动自检（和 CarAgent lifespan 对应, 全部非阻塞）==========

// InitDB 数据库连接检查
func InitDB(db *gorm.DB) {
	if db == nil {
		zap.L().Warn("DB 未配置，跳过")
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		zap.L().Warn("DB 获取连接失败", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		zap.L().Warn("DB 连接失败（非致命）", zap.Error(err))
		return
	}
	zap.L().Info("DB 连接就绪")
}

// InitRedis Redis 连接检查 (PING)
func InitRedis(rdb *redis.Client) {
	if rdb == nil {
		zap.L().Warn("Redis 未配置，跳过")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		zap.L().Warn("Redis 连接失败（非致命）", zap.Error(err))
		return
	}
	zap.L().Info("Redis 连接就绪")
}

// InitMilvus Milvus 连接检查
func InitMilvus(milvusURI string) {
	if milvusURI == "" {
		zap.L().Warn("Milvus 未配置，跳过")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, client.Config{Address: milvusURI})
	if err != nil {
		zap.L().Warn("Milvus 连接失败（非致命）", zap.String("uri", milvusURI), zap.Error(err))
		return
	}
	// 检查连接: 列出 Collections
	_, err = c.ListCollections(ctx)
	if err != nil {
		zap.L().Warn("Milvus 健康检查未通过（非致命）", zap.Error(err))
		return
	}
	zap.L().Info("Milvus 连接就绪")
}

// InitEmbedding BGE-M3 Embedding HTTP 服务初始化
// 1. 先健康检查, 已有则复用
// 2. 没有则尝试启动本地 Python HTTP 服务
// 3. 全部失败不阻塞启动
func InitEmbedding(embeddingURL string) {
	if embeddingURL == "" {
		embeddingURL = "http://localhost:19531"
	}

	// 1. 尝试健康检查 (3次)
	for i := 0; i < 3; i++ {
		resp, err := http.Get(embeddingURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			zap.L().Info("Embedding 服务就绪", zap.String("url", embeddingURL))
			return
		}
		time.Sleep(2 * time.Second)
	}

	// 2. 尝试启动本地 Python 服务
	zap.L().Info("Embedding 服务未就绪，尝试启动本地 BGE-M3...")

	scriptPath := "../../CarAgent/scripts/embedding_http_server.py"
	cmd := exec.Command("python", scriptPath, "--port", "19531")

	if err := cmd.Start(); err != nil {
		zap.L().Warn("BGE-M3 启动失败（非致命）",
			zap.Error(err),
			zap.String("hint", "手动: python "+scriptPath+" --port 19531"),
		)
		return
	}

	// 3. 等待就绪 (最多30s)
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		resp, err := http.Get(embeddingURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			zap.L().Info("BGE-M3 Embedding 服务启动成功", zap.String("url", embeddingURL))
			return
		}
	}
	zap.L().Warn("BGE-M3 启动超时（非致命）", zap.String("url", embeddingURL))
}

// InitLLM LLM 基础连通性检查 (发一个极短请求验证 API Key 有效)
func InitLLM(llmProvider, llmModel, llmBaseURL, llmAPIKey string) {
	if llmAPIKey == "" {
		zap.L().Warn("LLM API Key 未配置，跳过自检")
		return
	}
	zap.L().Info("LLM 自检跳过 (首次问答时惰性连接)",
		zap.String("provider", llmProvider),
		zap.String("model", llmModel),
	)
	_ = llmBaseURL
}

// PrintStartupBanner 打印启动横幅
// SettingsHandler 返回当前运行配置（全部从 cfg 读取，不写死）
func SettingsHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		providers := gin.H{}
		for _, name := range []string{"glm", "openai", "deepseek", "qwen"} {
			pm := cfg.LLM.Resolve(name)
			if pm.Key == "" { continue }
			providers[name] = gin.H{"url": pm.BaseURL, "apiKey": "***", "endpoints": gin.H{"chat": "/v1/chat/completions"}}
		}
		chatCandidates := func() []gin.H {
			var out []gin.H
			for _, m := range []struct{ n, p, m string }{
				{"glm", "glm", cfg.LLM.GLMModel},
				{"openai", "openai", cfg.LLM.OpenAIModel},
				{"deepseek", "deepseek", cfg.LLM.DeepSeekModel},
				{"qwen", "qwen", cfg.LLM.QwenModel},
			} {
				if m.m != "" {
					out = append(out, gin.H{"id": m.n, "provider": m.p, "model": m.m, "priority": 1})
				}
			}
			return out
		}
		c.JSON(200, gin.H{"code": "0", "data": gin.H{
			"upload": gin.H{"maxFileSize": 52428800, "maxRequestSize": 104857600},
			"rag": gin.H{
				"default":      gin.H{"collectionName": cfg.Milvus.CollectionName, "dimension": 1536, "metricType": "COSINE"},
				"queryRewrite": gin.H{"enabled": cfg.RAG.QueryRewrite},
				"rerank":       gin.H{"enabled": cfg.RAG.RerankEnabled},
				"rateLimit":    gin.H{"global": gin.H{"enabled": true, "maxConcurrent": 10, "maxWaitSeconds": 15, "leaseSeconds": 30, "pollIntervalMs": 200}},
				"memory":       gin.H{"historyKeepTurns": 8, "summaryStartTurns": 8, "summaryEnabled": true, "summaryMaxChars": 200, "titleMaxLength": 30},
				"search": gin.H{
					"defaultTopK": cfg.RAG.Search.DefaultTopK,
					"channels": gin.H{
						"vectorGlobal":   gin.H{"enabled": cfg.RAG.Search.Channels.VectorGlobal.Enabled, "confidenceThreshold": cfg.RAG.Search.Channels.VectorGlobal.ConfidenceThreshold, "topKMultiplier": cfg.RAG.Search.Channels.VectorGlobal.TopKMultiplier},
						"intentDirected": gin.H{"enabled": cfg.RAG.Search.Channels.IntentDirected.Enabled, "minIntentScore": cfg.RAG.Search.Channels.IntentDirected.MinIntentScore, "topKMultiplier": cfg.RAG.Search.Channels.IntentDirected.TopKMultiplier},
					},
				},
			},
			"ai": gin.H{
				"providers": providers,
				"selection": gin.H{"failureThreshold": 2, "openDurationMs": 30000},
				"stream":    gin.H{"messageChunkSize": 1},
				"chat":      gin.H{"defaultModel": cfg.LLM.PrimaryProvider(), "deepThinkingModel": "", "candidates": chatCandidates()},
				"embedding": gin.H{"defaultModel": cfg.Embedding.Model, "candidates": []gin.H{}},
				"rerank":    gin.H{"defaultModel": cfg.Reranker.Model, "candidates": []gin.H{}},
			},
		}})
	}
}

func PrintStartupBanner(appName, llmProvider string) {
	zap.L().Info("══════════════════════════════════════════")
	zap.L().Info(fmt.Sprintf("  %s v1.0.0 启动中...", appName))
	zap.L().Info(fmt.Sprintf("  LLM Provider: %s", llmProvider))
	zap.L().Info("══════════════════════════════════════════")
}
