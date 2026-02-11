package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	log   *zap.Logger
	sugar *zap.SugaredLogger
)

// Init 初始化日志
func Init(level string, development bool) error {
	// 解析日志级别
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// 配置
	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapLevel),
		Development: development,
		Encoding:    "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	// 创建 logger
	var err error
	log, err = config.Build(zap.AddCallerSkip(1))
	if err != nil {
		return err
	}

	sugar = log.Sugar()
	return nil
}

// L 获取 logger
func L() *zap.Logger {
	if log == nil {
		// 如果未初始化，使用默认配置
		_ = Init("info", false)
	}
	return log
}

// S 获取 sugared logger
func S() *zap.SugaredLogger {
	if sugar == nil {
		// 如果未初始化，使用默认配置
		_ = Init("info", false)
	}
	return sugar
}

// Sync 同步日志
func Sync() error {
	if log != nil {
		return log.Sync()
	}
	return nil
}

// With 创建带字段的 logger
func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	L().Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	L().Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

// Fatal 致命错误日志
func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
	os.Exit(1)
}
