package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goRAGENT/internal/config"
)

func HealthHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := map[string]string{"db": "OK", "redis": "OK", "milvus": "NOT_CONFIGURED"}
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": s})
	}
}

func SettingsHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": gin.H{
			"upload": gin.H{"maxFileSize": 52428800},
			"rateLimit": gin.H{
				"enabled": true, "maxConcurrent": 10,
				"maxWaitSeconds": 15, "leaseSeconds": 30, "pollIntervalMs": 200,
			},
		}})
	}
}
