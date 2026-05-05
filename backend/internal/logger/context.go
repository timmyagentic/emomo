package logger

import (
	"context"
	"sync"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey struct{}

// loggerKey is the key used to store logger in context
var loggerKey = contextKey{}

// defaultLogger is used when no logger is found in context
var (
	defaultLogger   *Logger
	defaultLoggerMu sync.RWMutex
)

func init() {
	defaultLogger = New(nil)
}

// ============================================
// Default Logger Access
// ============================================

// GetDefault returns the default logger (thread-safe).
// Use this when you need a logger outside of a context.
func GetDefault() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// getDefaultLogger is an internal function to get the default logger.
// Used by Entry API and other internal modules.
func getDefaultLogger() *Logger {
	return GetDefault()
}

// SetDefaultLogger sets the default logger used when no logger is found in context.
// Parameters:
//   - l: logger to set as default.
//
// Returns: none.
func SetDefaultLogger(l *Logger) {
	if l != nil {
		defaultLoggerMu.Lock()
		defaultLogger = l
		defaultLoggerMu.Unlock()
	}
}

// ============================================
// Context Logger Access
// ============================================

// WithContext returns a new context with the logger attached.
// Parameters:
//   - ctx: existing context to wrap.
//
// Returns:
//   - context.Context: context containing the logger.
func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext extracts the logger from context.
// Parameters:
//   - ctx: context to inspect.
//
// Returns:
//   - *Logger: logger with injected fields or the default logger.
func FromContext(ctx context.Context) *Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey).(*Logger); ok {
			return l
		}
	}
	defaultLoggerMu.RLock()
	l := defaultLogger
	defaultLoggerMu.RUnlock()
	return l
}

// ============================================
// Context Field Injection
// ============================================

// WithField creates a new context with a single additional field.
// This is an alias for ContextWithField with a shorter name.
func WithField(ctx context.Context, key string, value interface{}) context.Context {
	l := FromContext(ctx).WithField(key, value)
	return l.WithContext(ctx)
}

// WithFields creates a new context with additional fields added to the logger.
// This is an alias for ContextWithFields with a shorter name.
func WithFields(ctx context.Context, fields Fields) context.Context {
	l := FromContext(ctx).WithFields(fields)
	return l.WithContext(ctx)
}

// ContextWithFields creates a new context with additional fields added to the logger.
// Parameters:
//   - ctx: base context.
//   - fields: structured fields to add.
//
// Returns:
//   - context.Context: context containing the enriched logger.
func ContextWithFields(ctx context.Context, fields Fields) context.Context {
	return WithFields(ctx, fields)
}

// ContextWithField creates a new context with a single additional field.
// Parameters:
//   - ctx: base context.
//   - key: field key.
//   - value: field value.
//
// Returns:
//   - context.Context: context containing the enriched logger.
func ContextWithField(ctx context.Context, key string, value interface{}) context.Context {
	return WithField(ctx, key, value)
}

// ============================================
// Standard Field Setters
// ============================================

// SetRequestID sets the request ID field in context.
func SetRequestID(ctx context.Context, id string) context.Context {
	return WithField(ctx, FieldRequestID, id)
}

// SetJobID sets the job ID field in context.
func SetJobID(ctx context.Context, id string) context.Context {
	return WithField(ctx, FieldJobID, id)
}

// SetSearchID sets the search ID field in context.
func SetSearchID(ctx context.Context, id string) context.Context {
	return WithField(ctx, FieldSearchID, id)
}

// SetComponent sets the component name field in context.
func SetComponent(ctx context.Context, name string) context.Context {
	return WithField(ctx, FieldComponent, name)
}

// SetSource sets the source field in context.
func SetSource(ctx context.Context, source string) context.Context {
	return WithField(ctx, FieldSource, source)
}

// ============================================
// Field Extraction
// ============================================

// GetField extracts a field value from the context's logger.
func GetField(ctx context.Context, key string) (interface{}, bool) {
	log := FromContext(ctx)
	val, ok := log.Data[key]
	return val, ok
}

// GetFieldString extracts a string field value from the context's logger.
func GetFieldString(ctx context.Context, key string) string {
	val, ok := GetField(ctx, key)
	if !ok {
		return ""
	}
	str, _ := val.(string)
	return str
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	return GetFieldString(ctx, FieldRequestID)
}

// GetJobID extracts the job ID from context.
func GetJobID(ctx context.Context) string {
	return GetFieldString(ctx, FieldJobID)
}

// GetSearchID extracts the search ID from context.
func GetSearchID(ctx context.Context) string {
	return GetFieldString(ctx, FieldSearchID)
}

// GetComponent extracts the component name from context.
func GetComponent(ctx context.Context) string {
	return GetFieldString(ctx, FieldComponent)
}

// GetSource extracts the source from context.
func GetSource(ctx context.Context) string {
	return GetFieldString(ctx, FieldSource)
}

// GetFields extracts all fields from the context's logger.
func GetFields(ctx context.Context) Fields {
	log := FromContext(ctx)
	fields := make(Fields, len(log.Data))
	for k, v := range log.Data {
		fields[k] = v
	}
	return fields
}
