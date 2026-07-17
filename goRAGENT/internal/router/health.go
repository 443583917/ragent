package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
)

// HealthHandler 返回基础健康检查结果。
func HealthHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := map[string]string{"db": "OK", "redis": "OK", "milvus": "NOT_CONFIGURED"}
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": s})
	}
}

// SettingsHandler 返回前端运行配置（全部从 cfg 读取，不写死）。
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
