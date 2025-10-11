package logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level  string
	Format string
}

type contextKey struct{}

const (
	formatJSON    = "json"
	formatConsole = "console"
)

func New(cfg Config) (*zap.Logger, error) {
	zapCfg, err := buildZapConfig(cfg)
	if err != nil {
		return nil, err
	}
	return zapCfg.Build()
}

func buildZapConfig(cfg Config) (zap.Config, error) {
	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	var zapCfg zap.Config
	switch format {
	case formatJSON:
		zapCfg = zap.NewProductionConfig()
	case formatConsole:
		zapCfg = zap.NewDevelopmentConfig()
	default:
		return zap.Config{}, fmt.Errorf("logger: unsupported format %s", cfg.Format)
	}
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return zap.Config{}, err
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.Encoding = format
	encoder := buildEncoderConfig(format)
	zapCfg.EncoderConfig = encoder
	zapCfg.OutputPaths = []string{"stdout"}
	zapCfg.ErrorOutputPaths = []string{"stderr"}
	return zapCfg, nil
}

func buildEncoderConfig(format string) zapcore.EncoderConfig {
	encoder := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	if format == formatConsole {
		if isCI() {
			encoder.EncodeLevel = zapcore.CapitalLevelEncoder
			encoder.EncodeTime = zapcore.RFC3339TimeEncoder
		} else {
			encoder.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}
	}
	return encoder
}

func isCI() bool {
	ciEnvVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"CIRCLECI",
		"TRAVIS",
		"JENKINS_URL",
		"BUILDKITE",
		"DRONE",
		"TEAMCITY_VERSION",
	}
	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	}
	return zapcore.InfoLevel, fmt.Errorf("logger: unsupported level %s", level)
}

func IntoContext(ctx context.Context, log *zap.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, log)
}

func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.NewNop()
	}
	log, ok := ctx.Value(contextKey{}).(*zap.Logger)
	if !ok || log == nil {
		return zap.NewNop()
	}
	return log
}

func With(ctx context.Context, fields ...zap.Field) *zap.Logger {
	return FromContext(ctx).With(fields...)
}

func Sync(log *zap.Logger) error {
	if log == nil {
		return nil
	}
	err := log.Sync()
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	if errors.Is(err, syscall.EINVAL) {
		return nil
	}
	if errors.Is(err, syscall.EBADF) {
		return nil
	}
	return err
}
