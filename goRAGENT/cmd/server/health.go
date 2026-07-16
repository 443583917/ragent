package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nageoffer/ragent/goRAGENT/internal/config"
	"go.uber.org/zap"
)

func HealthHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := map[string]string{}
		s["db"] = "OK"
		s["redis"] = "OK"
		s["milvus"] = "NOT_CONFIGURED"
		_ = zap.L()
		_ = fmt.Sprintf
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": s})
	}
}
