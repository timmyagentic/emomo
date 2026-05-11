package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// writerCloser holds a reference to closable writers for Sync()
var (
	writerCloser   io.Closer
	writerCloserMu sync.Mutex
)

// Logger wraps logrus.Entry to provide structured logging with context support.
type Logger struct {
	*logrus.Entry
}

// Config holds logger configuration.
type Config struct {
	Level       string    // debug, info, warn, error
	Format      string    // json, text
	Output      io.Writer // output destination
	ServiceName string    // service name for log tagging
}

// DefaultConfig returns sensible defaults.
// Parameters: none.
// Returns:
//   - *Config: default logger configuration.
func DefaultConfig() *Config {
	return &Config{
		Level:       "info",
		Format:      "json",
		Output:      os.Stdout,
		ServiceName: "emomo",
	}
}

// New creates a new Logger with the given configuration.
// Parameters:
//   - cfg: logger configuration; nil uses DefaultConfig.
//
// Returns:
//   - *Logger: initialized logger instance.
func New(cfg *Config) *Logger {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	log := logrus.New()

	// Set output
	if cfg.Output != nil {
		log.SetOutput(cfg.Output)
	} else {
		log.SetOutput(os.Stdout)
	}

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	// Enable caller reporting
	log.SetReportCaller(true)

	// Set formatter - JSON format as default
	if cfg.Format == "text" {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:    true,
			TimestampFormat:  "2006-01-02T15:04:05.000Z07:00",
			CallerPrettyfier: callerPrettyfier,
		})
	} else {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
			CallerPrettyfier: callerPrettyfier,
		})
	}

	// Create base entry with service name
	entry := log.WithField("service", cfg.ServiceName)

	return &Logger{Entry: entry}
}

// NewFromEnv creates a new Logger from environment configuration.
// This supports log rotation and multi-output (stdout + file).
func NewFromEnv(envCfg *EnvConfig) *Logger {
	if envCfg == nil {
		envCfg = LoadFromEnv()
	}

	log := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(envCfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	// Enable caller reporting
	log.SetReportCaller(true)

	// Set formatter
	if strings.ToLower(envCfg.Format) == "text" {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:    true,
			TimestampFormat:  "2006-01-02T15:04:05.000Z07:00",
			CallerPrettyfier: callerPrettyfier,
		})
	} else {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
			CallerPrettyfier: callerPrettyfier,
		})
	}

	// Configure output
	if envCfg.Output != nil {
		// Use explicitly specified output
		log.SetOutput(envCfg.Output)
	} else {
		// Configure based on environment
		var writers []io.Writer

		// Stdout output (for local env or when not file-only)
		if envCfg.Environment == "local" || !envCfg.LogFileOnly {
			writers = append(writers, os.Stdout)
		}

		// File output (for non-local environments)
		if envCfg.Environment != "local" && envCfg.LogFile != "" {
			fileWriter := &lumberjack.Logger{
				Filename:   envCfg.LogFile,
				MaxSize:    envCfg.MaxSize, // MB
				MaxBackups: envCfg.MaxBackups,
				MaxAge:     envCfg.MaxAge, // days
				Compress:   envCfg.Compress,
			}
			writers = append(writers, fileWriter)

			// Store closer reference for Sync()
			writerCloserMu.Lock()
			writerCloser = fileWriter
			writerCloserMu.Unlock()
		}

		if len(writers) == 0 {
			writers = append(writers, os.Stdout)
		}

		log.SetOutput(io.MultiWriter(writers...))
	}

	// Create base entry with service name
	entry := log.WithField("service", envCfg.ServiceName)

	return &Logger{Entry: entry}
}

// NewDefault creates a new Logger using environment variable configuration.
// This is the recommended way to create a logger in main().
func NewDefault() *Logger {
	return NewFromEnv(nil)
}

// NewServiceFromEnv creates a new Logger from environment configuration while
// overriding the service field for a specific entry point.
func NewServiceFromEnv(serviceName string) *Logger {
	envCfg := LoadFromEnv()
	if serviceName != "" {
		envCfg.ServiceName = serviceName
	}
	return NewFromEnv(envCfg)
}

// OpenRotatingFile opens (creating any missing parent directories) a
// lumberjack-backed rotating log file at path and returns it as an
// io.WriteCloser. It is intended for CLI entry points (e.g. cmd/ingest) that
// want a persistent log file alongside stdout without going through the
// env-driven NewFromEnv path.
//
// The default rotation policy mirrors NewFromEnv: 100MB per file, up to 7
// gzipped backups, keeping at most 30 days of history.
//
// Callers MUST Close() the returned writer before the process exits so the
// final log batch is flushed to disk; logger.Sync() does not own this writer.
func OpenRotatingFile(path string) (io.WriteCloser, error) {
	if path == "" {
		return nil, fmt.Errorf("logger: rotating file path is empty")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("logger: failed to create log directory %s: %w", dir, err)
		}
	}
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    100,
		MaxBackups: 7,
		MaxAge:     30,
		Compress:   true,
	}, nil
}

// Sync flushes all pending logs and closes file handles.
// Should be called before program exit to ensure no logs are lost.
//
// Usage:
//
//	func main() {
//	    logger.SetDefaultLogger(logger.NewDefault())
//	    defer logger.Sync()
//	    // ...
//	}
func Sync() error {
	writerCloserMu.Lock()
	defer writerCloserMu.Unlock()

	if writerCloser != nil {
		return writerCloser.Close()
	}
	return nil
}

// WithFields returns a new Logger with additional fields.
// Parameters:
//   - fields: structured fields to add.
//
// Returns:
//   - *Logger: derived logger with fields applied.
func (l *Logger) WithFields(fields Fields) *Logger {
	return &Logger{Entry: l.Entry.WithFields(logrus.Fields(fields))}
}

// WithField returns a new Logger with a single additional field.
// Parameters:
//   - key: field key.
//   - value: field value.
//
// Returns:
//   - *Logger: derived logger with field applied.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{Entry: l.Entry.WithField(key, value)}
}

// WithError returns a new Logger with an error field.
// Parameters:
//   - err: error to attach.
//
// Returns:
//   - *Logger: derived logger with error field.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{Entry: l.Entry.WithError(err)}
}

// callerPrettyfier simplifies caller information to show only relative path and line number
func callerPrettyfier(frame *runtime.Frame) (function string, file string) {
	// Get short function name (without package path)
	funcName := frame.Function
	if idx := strings.LastIndex(funcName, "/"); idx != -1 {
		funcName = funcName[idx+1:]
	}

	// Get short file path (only filename:line)
	fileName := filepath.Base(frame.File)

	return funcName, fileName + ":" + itoa(frame.Line)
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}

// ============================================
// Simple Log Functions (no Context)
// ============================================

// Debug logs a message at Debug level.
func Debug(format string, args ...interface{}) {
	getDefaultLogger().Debugf(format, args...)
}

// Info logs a message at Info level.
func Info(format string, args ...interface{}) {
	getDefaultLogger().Infof(format, args...)
}

// Warn logs a message at Warn level.
func Warn(format string, args ...interface{}) {
	getDefaultLogger().Warnf(format, args...)
}

// Error logs a message at Error level.
func Error(format string, args ...interface{}) {
	getDefaultLogger().Errorf(format, args...)
}

// Fatal logs a message at Fatal level and exits.
func Fatal(format string, args ...interface{}) {
	getDefaultLogger().Fatalf(format, args...)
}

// ============================================
// Context Log Functions (recommended)
// ============================================

// CtxDebug logs a message at Debug level with context fields.
func CtxDebug(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Debugf(format, args...)
}

// CtxInfo logs a message at Info level with context fields.
func CtxInfo(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Infof(format, args...)
}

// CtxWarn logs a message at Warn level with context fields.
func CtxWarn(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Warnf(format, args...)
}

// CtxError logs a message at Error level with context fields.
func CtxError(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Errorf(format, args...)
}

// CtxFatal logs a message at Fatal level with context fields and exits.
func CtxFatal(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Fatalf(format, args...)
}
