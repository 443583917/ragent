package router

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"goRAGENT/internal/config"
)

// healthStatus 运行时健康状态（bootstrap 探活后注入）。
type healthStatus struct {
	db      *gorm.DB
	rdb     *redis.Client
	milvusURI string
}

// HealthHandler 返回运行时健康检查（实际探测 DB/Redis/Milvus）。
func HealthHandler(hs healthStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := map[string]string{"db": "OK", "redis": "OK", "milvus": "OK"}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if hs.db != nil {
			if sqlDB, err := hs.db.DB(); err == nil {
				if err := sqlDB.PingContext(ctx); err != nil {
					s["db"] = "DOWN"
				}
			} else {
				s["db"] = "DOWN"
			}
		} else {
			s["db"] = "NOT_CONFIGURED"
		}

		if hs.rdb != nil {
			if err := hs.rdb.Ping(ctx).Err(); err != nil {
				s["redis"] = "DOWN"
			}
		} else {
			s["redis"] = "NOT_CONFIGURED"
		}

		if hs.milvusURI != "" {
			origCtx, origCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer origCancel()
			if err := probeMilvusQuick(origCtx, hs.milvusURI); err != nil {
				s["milvus"] = "DOWN"
			}
		} else {
			s["milvus"] = "NOT_CONFIGURED"
		}

		c.JSON(http.StatusOK, gin.H{"code": "0", "data": s})
	}
}

// probeMilvusQuick 快速 Milvus 连通性检查（无 SDK 创建 client 开销的简化版）。
func probeMilvusQuick(ctx context.Context, uri string) error {
	// 直接用 HTTP HEAD 检查 Milvus RESTful API
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri+"/api/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SettingsHandler 返回前端运行配置（全部从 cfg 读取）。
func SettingsHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		providers := gin.H{}
		for _, name := range []string{"glm", "openai", "deepseek", "qwen"} {
			pm := cfg.LLM.Resolve(name)
			if pm.Key == "" {
				continue
			}
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
