package logger

import (
	"github.com/gumeniukcom/contactshq/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a *zap.Logger from LogConfig.
// format "json" uses production config, "text" uses development config.
// level is parsed dynamically (debug, info, warn, error, fatal).
func New(cfg config.LogConfig) (*zap.Logger, error) {
	var zapCfg zap.Config

	switch cfg.Format {
	case "text":
		zapCfg = zap.NewDevelopmentConfig()
	default:
		zapCfg = zap.NewProductionConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	return zapCfg.Build()
}
