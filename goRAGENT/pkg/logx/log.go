package logx

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化 zap 日志器
// mode: "debug" → 开发模式 (console, 彩色), 其他 → 生产模式 (JSON)
func Init(mode string) *zap.Logger {
	var cfg zap.Config

	if mode == "debug" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		panic("初始化日志器失败: " + err.Error())
	}

	return logger
}

// WithField 创建携带单字段的子日志器
func WithField(logger *zap.Logger, key string, val interface{}) *zap.Logger {
	return logger.With(zap.Any(key, val))
}
