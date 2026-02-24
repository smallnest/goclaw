package logger

import (
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogLevel represents log level
type LogLevel int

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in production
	DebugLevel LogLevel = iota
	// InfoLevel is the default logging priority
	InfoLevel
	// WarnLevel logs are more important than Info, but don't need individual human review
	WarnLevel
	// ErrorLevel logs are high-priority. If an application is running smoothly, it shouldn't generate any error-level logs
	ErrorLevel
	// FatalLevel logs. After a fatal log, the application will exit
	FatalLevel
)

var (
	level    *zap.AtomicLevel
	levelMux sync.RWMutex
)

// SetLevel changes the log level dynamically
func SetLevel(l LogLevel) {
	levelMux.Lock()
	defer levelMux.Unlock()

	if level == nil {
		// Initialize level if not set
		newLevel := zap.NewAtomicLevel()
		level = &newLevel
	}

	var zapLevel zapcore.Level
	switch l {
	case DebugLevel:
		zapLevel = zapcore.DebugLevel
	case InfoLevel:
		zapLevel = zapcore.InfoLevel
	case WarnLevel:
		zapLevel = zapcore.WarnLevel
	case ErrorLevel:
		zapLevel = zapcore.ErrorLevel
	case FatalLevel:
		zapLevel = zapcore.FatalLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	level.SetLevel(zapLevel)
	Info("Log level changed",
		zap.String("new_level", zapLevel.String()))
}

// GetLevel gets the current log level
func GetLevel() LogLevel {
	levelMux.RLock()
	defer levelMux.RUnlock()

	if level == nil {
		return InfoLevel
	}

	zapLevel := level.Level()
	switch zapLevel {
	case zapcore.DebugLevel:
		return DebugLevel
	case zapcore.InfoLevel:
		return InfoLevel
	case zapcore.WarnLevel:
		return WarnLevel
	case zapcore.ErrorLevel:
		return ErrorLevel
	case zapcore.FatalLevel:
		return FatalLevel
	default:
		return InfoLevel
	}
}

// ParseLevel parses a string to a log level
func ParseLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "INFO"
	}
}

// FieldLogger provides structured logging with fields
type FieldLogger struct {
	logger *zap.Logger
	fields []zap.Field
}

// NewFieldLogger creates a new field logger
func NewFieldLogger(fields ...zap.Field) *FieldLogger {
	return &FieldLogger{
		logger: L(),
		fields: fields,
	}
}

// With creates a child logger with additional fields
func (fl *FieldLogger) With(fields ...zap.Field) *FieldLogger {
	return &FieldLogger{
		logger: fl.logger,
		fields: append(fl.fields, fields...),
	}
}

// Debug logs at debug level
func (fl *FieldLogger) Debug(msg string, fields ...zap.Field) {
	allFields := append(fl.fields, fields...)
	fl.logger.Debug(msg, allFields...)
}

// Info logs at info level
func (fl *FieldLogger) Info(msg string, fields ...zap.Field) {
	allFields := append(fl.fields, fields...)
	fl.logger.Info(msg, allFields...)
}

// Warn logs at warn level
func (fl *FieldLogger) Warn(msg string, fields ...zap.Field) {
	allFields := append(fl.fields, fields...)
	fl.logger.Warn(msg, allFields...)
}

// Error logs at error level
func (fl *FieldLogger) Error(msg string, fields ...zap.Field) {
	allFields := append(fl.fields, fields...)
	fl.logger.Error(msg, allFields...)
}

// Fatal logs at fatal level and exits
func (fl *FieldLogger) Fatal(msg string, fields ...zap.Field) {
	allFields := append(fl.fields, fields...)
	fl.logger.Fatal(msg, allFields...)
}

// Module creates a field logger with module name
func Module(name string) *FieldLogger {
	return NewFieldLogger(zap.String("module", name))
}

// Request creates a field logger for request tracking
func Request(id string) *FieldLogger {
	return NewFieldLogger(zap.String("request_id", id))
}

// Operation creates a field logger for operation tracking
func Operation(name string) *FieldLogger {
	return NewFieldLogger(zap.String("operation", name))
}

// Component creates a field logger for component tracking
func Component(name string) *FieldLogger {
	return NewFieldLogger(zap.String("component", name))
}
