package main

import (
	"log"
	"os"
	"strings"

	"go.uber.org/zap"

	"goRAGENT/internal/bootstrap"
	"goRAGENT/internal/config"
	"goRAGENT/pkg/logx"
	"goRAGENT/pkg/snowflake"
)

// loadDotEnv 加载 .env 文件（仅设置未被系统环境变量覆盖的键）。
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

	// --migrate 分支：执行迁移后直接退出
	if HasMigrateFlag() {
		if err := runMigrations(cfg.MySQL.DSN()); err != nil {
			log.Fatal(err)
		}
		return
	}

	logger := logx.Init(strings.ToLower(cfg.Log.Level))
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	if err := snowflake.Init(1); err != nil {
		log.Fatalf("Snowflake: %v", err)
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	bootstrap.PrintStartupBanner(cfg.App.Name, cfg.LLM.PrimaryProvider())
	app.Run()
}
