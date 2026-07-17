package bootstrap

import (
	"context"
	"net/http"
	"os/exec"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ========== 启动自检（非阻塞）==========

// probeDB 数据库连接健康检查。
func probeDB(db *gorm.DB) {
	if db == nil {
		zap.L().Warn("DB 未配置，跳过自检")
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

// probeRedis Redis 连接健康检查 (PING)。
func probeRedis(rdb *redis.Client) {
	if rdb == nil {
		zap.L().Warn("Redis 未配置，跳过自检")
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

// probeMilvus Milvus 连接健康检查。
func probeMilvus(milvusURI string) {
	if milvusURI == "" {
		zap.L().Warn("Milvus 未配置，跳过自检")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, client.Config{Address: milvusURI})
	if err != nil {
		zap.L().Warn("Milvus 连接失败（非致命）", zap.String("uri", milvusURI), zap.Error(err))
		return
	}
	_, err = c.ListCollections(ctx)
	if err != nil {
		zap.L().Warn("Milvus 健康检查未通过（非致命）", zap.Error(err))
		return
	}
	zap.L().Info("Milvus 连接就绪")
}

// probeEmbedding BGE-M3 Embedding HTTP 服务初始化。
// 先健康检查，没有则尝试启动本地 Python 服务，全部失败不阻塞启动。
func probeEmbedding(embeddingURL string) {
	if embeddingURL == "" {
		embeddingURL = "http://localhost:19531"
	}

	// 1. 健康检查 (3次)
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

// probeLLM LLM 基础连通性检查。
func probeLLM(llmProvider, llmModel, llmBaseURL, llmAPIKey string) {
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
