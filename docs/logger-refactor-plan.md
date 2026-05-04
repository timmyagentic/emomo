# Emomo Logger 重构计划

## 1. 现状分析

### 1.1 现有实现优势

✅ **已具备的核心能力：**
- 基于 logrus 的结构化日志（JSON 格式）
- Context 集成机制（`FromContext`, `WithContext`）
- Fields 支持 (`WithFields`)
- HTTP 请求 ID 追踪（UUID）
- 延迟测量（毫秒级）
- 调用者信息自动报告
- 服务名标识（`ServiceName`）

✅ **日志覆盖完整：**
- 196 处日志调用
- 涵盖 API、摄取、搜索等核心业务流程
- 清晰的日志级别区分（Info/Warn/Error/Debug）

### 1.2 需要改进的方面

❌ **缺失的关键特性：**

| 特性 | 现状 | 影响 |
|------|------|------|
| **追踪字段标准化** | 无 `task_id`, `trace_id`, `component` 标准 | 难以跨服务关联日志 |
| **Context 字段注入** | 需要手动 `WithFields` | 使用繁琐，容易遗漏 |
| **指标/描述分离** | 混用 Fields 和 message | 查询效率低，难以聚合 |
| **简化 API** | 无 `CtxInfo/CtxError` 等便捷函数 | 代码冗长 |
| **环境变量配置** | 仅支持代码配置 | 部署不灵活 |
| **日志轮转** | 无文件轮转支持 | 磁盘可能打满 |
| **输出控制** | 无 stdout/file 分离 | 开发/生产环境体验差 |

---

## 2. 重构目标

### 2.1 核心设计原则

**三层分类设计：**

```
┌─────────────────┬──────────────────┬─────────────────────────┐
│     类型        │    存储位置      │         用途            │
├─────────────────┼──────────────────┼─────────────────────────┤
│ **追踪字段**    │ Context → JSON   │ 贯穿调用链，日志关联    │
│ (task_id)       │                  │ 示例: search_id, job_id │
├─────────────────┼──────────────────┼─────────────────────────┤
│ **指标字段**    │ Entry → JSON     │ 单次日志，聚合/告警     │
│ (duration_ms)   │                  │ 示例: count, size       │
├─────────────────┼──────────────────┼─────────────────────────┤
│ **描述信息**    │ message 字符串   │ 人类阅读                │
│ (url, path)     │                  │ 示例: url=%s, step=%s   │
└─────────────────┴──────────────────┴─────────────────────────┘
```

**判断标准：** 这个字段需要被机器查询/聚合/告警吗？
- **是** → JSON 字段（Context 或 Entry）
- **否** → message 字符串

### 2.2 API 设计目标

```go
// ✅ 理想的使用方式
func IngestMemes(ctx context.Context, source string) error {
    // 1. 入口处注入追踪字段（一次性）
    ctx = logger.WithFields(ctx, logger.Fields{
        logger.FieldComponent: "ingest",
        logger.FieldJobID:     uuid.New().String(),
    })

    // 2. 业务日志简洁清晰
    logger.CtxInfo(ctx, "Starting ingestion: source=%s", source)

    startTime := time.Now()
    err := process(ctx)
    duration := time.Since(startTime)

    // 3. 指标字段单独记录（可聚合）
    if err != nil {
        logger.With(logger.Fields{
            "duration_ms": duration.Milliseconds(),
        }).Error(ctx, "Ingestion failed: source=%s, error=%v", source, err)
        return err
    }

    logger.With(logger.Fields{
        "duration_ms": duration.Milliseconds(),
    }).Info(ctx, "Ingestion completed: source=%s", source)
    return nil
}
```

---

## 3. 重构实施计划

### 阶段一：增强 Logger 核心能力 (Week 1)

#### 3.1 标准追踪字段定义

**文件：** `internal/logger/fields.go`

```go
package logger

// 标准追踪字段（Context 级别，贯穿调用链）
const (
    FieldRequestID = "request_id" // HTTP 请求 ID（已有）
    FieldJobID     = "job_id"     // 数据摄取任务 ID
    FieldSearchID  = "search_id"  // 搜索请求 ID
    FieldComponent = "component"  // 组件/模块名
    FieldSource    = "source"     // 数据源标识
    FieldUserID    = "user_id"    // 用户 ID（预留）

    // OpenTelemetry 分布式追踪字段
    FieldTraceID = "trace_id" // OpenTelemetry Trace ID
    FieldSpanID  = "span_id"  // OpenTelemetry Span ID
)

// 标准指标字段（Entry 级别，用于聚合）
const (
    FieldDurationMs = "duration_ms" // 执行耗时（毫秒）
    FieldCount      = "count"       // 计数
    FieldSize       = "size"        // 数据大小（字节）
    FieldStatus     = "status"      // 状态
)
```

#### 3.2 Context 字段便捷操作

**文件：** `internal/logger/context.go`（扩展现有文件）

```go
// ============================================
// Default Logger 获取函数
// ============================================

// GetDefault 获取默认 logger（线程安全）
// 用于在没有 context 的场景下获取 logger
func GetDefault() *Logger {
    defaultLoggerMu.RLock()
    defer defaultLoggerMu.RUnlock()
    return defaultLogger
}

// getDefaultLogger 内部使用的获取函数
// Entry API 等内部模块使用此函数
func getDefaultLogger() *Logger {
    return GetDefault()
}

// ============================================
// Context 字段注入函数
// ============================================

// 便捷的单字段注入函数
func WithField(ctx context.Context, key string, value interface{}) context.Context {
    log := FromContext(ctx)
    return log.WithField(key, value).WithContext(ctx)
}

// 批量字段注入（已有 ContextWithFields，可重命名为 WithFields）
func WithFields(ctx context.Context, fields Fields) context.Context {
    return ContextWithFields(ctx, fields)
}

// 标准字段的快捷注入函数
func SetRequestID(ctx context.Context, id string) context.Context {
    return WithField(ctx, FieldRequestID, id)
}

func SetJobID(ctx context.Context, id string) context.Context {
    return WithField(ctx, FieldJobID, id)
}

func SetSearchID(ctx context.Context, id string) context.Context {
    return WithField(ctx, FieldSearchID, id)
}

func SetComponent(ctx context.Context, name string) context.Context {
    return WithField(ctx, FieldComponent, name)
}

func SetSource(ctx context.Context, source string) context.Context {
    return WithField(ctx, FieldSource, source)
}

// 字段提取函数
func GetField(ctx context.Context, key string) (interface{}, bool) {
    log := FromContext(ctx)
    val, ok := log.Data[key]
    return val, ok
}

func GetFieldString(ctx context.Context, key string) string {
    val, ok := GetField(ctx, key)
    if !ok {
        return ""
    }
    str, _ := val.(string)
    return str
}

func GetRequestID(ctx context.Context) string {
    return GetFieldString(ctx, FieldRequestID)
}

func GetJobID(ctx context.Context) string {
    return GetFieldString(ctx, FieldJobID)
}

func GetSearchID(ctx context.Context) string {
    return GetFieldString(ctx, FieldSearchID)
}

func GetComponent(ctx context.Context) string {
    return GetFieldString(ctx, FieldComponent)
}

func GetFields(ctx context.Context) Fields {
    log := FromContext(ctx)
    fields := make(Fields, len(log.Data))
    for k, v := range log.Data {
        fields[k] = v
    }
    return fields
}

// ============================================
// OpenTelemetry 集成
// ============================================

import "go.opentelemetry.io/otel/trace"

// InjectTraceID 从 context 提取 OpenTelemetry trace/span ID 并注入 logger
// 用于将分布式追踪与日志关联，便于在 Grafana 等工具中跨服务追踪
//
// 使用示例（在 HTTP 中间件中）:
//
//	func TracingMiddleware() gin.HandlerFunc {
//	    return func(c *gin.Context) {
//	        ctx := c.Request.Context()
//	        ctx = logger.InjectTraceID(ctx)
//	        c.Request = c.Request.WithContext(ctx)
//	        c.Next()
//	    }
//	}
func InjectTraceID(ctx context.Context) context.Context {
    span := trace.SpanFromContext(ctx)
    if !span.SpanContext().IsValid() {
        return ctx
    }
    return WithFields(ctx, Fields{
        FieldTraceID: span.SpanContext().TraceID().String(),
        FieldSpanID:  span.SpanContext().SpanID().String(),
    })
}

// GetTraceID 从 context 获取 trace ID
func GetTraceID(ctx context.Context) string {
    return GetFieldString(ctx, FieldTraceID)
}

// GetSpanID 从 context 获取 span ID
func GetSpanID(ctx context.Context) string {
    return GetFieldString(ctx, FieldSpanID)
}
```

#### 3.3 Entry API（指标字段专用）

**文件：** `internal/logger/entry.go`（新建）

```go
package logger

import (
    "context"
)

// Entry 代表一个包含指标字段的日志条目
// 用于记录可聚合的指标（duration_ms, count, size 等）
type Entry struct {
    logger *Logger
    fields Fields
}

// With 创建一个包含指标字段的 Entry
// 示例: logger.With(logger.Fields{"duration_ms": 1234}).Info(ctx, "Task completed")
func With(fields Fields) *Entry {
    return &Entry{
        logger: getDefaultLogger(),
        fields: fields,
    }
}

// With 在现有 Entry 基础上添加更多字段
func (e *Entry) With(fields Fields) *Entry {
    merged := make(Fields, len(e.fields)+len(fields))
    for k, v := range e.fields {
        merged[k] = v
    }
    for k, v := range fields {
        merged[k] = v
    }
    return &Entry{
        logger: e.logger,
        fields: merged,
    }
}

// WithField 添加单个字段
func (e *Entry) WithField(key string, value interface{}) *Entry {
    return e.With(Fields{key: value})
}

// Info 记录 Info 级别日志
func (e *Entry) Info(ctx context.Context, format string, args ...interface{}) {
    log := e.logger
    if ctx != nil {
        log = FromContext(ctx)
    }
    log.WithFields(e.fields).Infof(format, args...)
}

// Warn 记录 Warn 级别日志
func (e *Entry) Warn(ctx context.Context, format string, args ...interface{}) {
    log := e.logger
    if ctx != nil {
        log = FromContext(ctx)
    }
    log.WithFields(e.fields).Warnf(format, args...)
}

// Error 记录 Error 级别日志
func (e *Entry) Error(ctx context.Context, format string, args ...interface{}) {
    log := e.logger
    if ctx != nil {
        log = FromContext(ctx)
    }
    log.WithFields(e.fields).Errorf(format, args...)
}

// Debug 记录 Debug 级别日志
func (e *Entry) Debug(ctx context.Context, format string, args ...interface{}) {
    log := e.logger
    if ctx != nil {
        log = FromContext(ctx)
    }
    log.WithFields(e.fields).Debugf(format, args...)
}

// Fatal 记录 Fatal 级别日志并退出
func (e *Entry) Fatal(ctx context.Context, format string, args ...interface{}) {
    log := e.logger
    if ctx != nil {
        log = FromContext(ctx)
    }
    log.WithFields(e.fields).Fatalf(format, args...)
}
```

#### 3.4 简化日志 API

**文件：** `internal/logger/logger.go`（扩展现有文件）

```go
// ============================================
// 简单日志函数（无 Context）
// ============================================

func Debug(format string, args ...interface{}) {
    getDefaultLogger().Debugf(format, args...)
}

func Info(format string, args ...interface{}) {
    getDefaultLogger().Infof(format, args...)
}

func Warn(format string, args ...interface{}) {
    getDefaultLogger().Warnf(format, args...)
}

func Error(format string, args ...interface{}) {
    getDefaultLogger().Errorf(format, args...)
}

func Fatal(format string, args ...interface{}) {
    getDefaultLogger().Fatalf(format, args...)
}

// ============================================
// Context 日志函数（推荐使用）
// ============================================

func CtxDebug(ctx context.Context, format string, args ...interface{}) {
    FromContext(ctx).Debugf(format, args...)
}

func CtxInfo(ctx context.Context, format string, args ...interface{}) {
    FromContext(ctx).Infof(format, args...)
}

func CtxWarn(ctx context.Context, format string, args ...interface{}) {
    FromContext(ctx).Warnf(format, args...)
}

func CtxError(ctx context.Context, format string, args ...interface{}) {
    FromContext(ctx).Errorf(format, args...)
}

func CtxFatal(ctx context.Context, format string, args ...interface{}) {
    FromContext(ctx).Fatalf(format, args...)
}
```

---

### 阶段二：配置增强 (Week 2)

#### 3.5 环境变量配置支持

**文件：** `internal/logger/config.go`（新建）

```go
package logger

import (
    "io"
    "os"
    "strconv"
    "strings"
)

// Config 日志配置
type Config struct {
    // 基础配置
    Level       string    // 日志级别: debug, info, warn, error
    Format      string    // 输出格式: json, text
    Output      io.Writer // 输出目标（优先级最高）
    ServiceName string    // 服务名称

    // 环境配置
    Environment string // 环境: local, dev, prod

    // 文件输出配置
    LogFile     string // 日志文件路径
    LogFileOnly bool   // 仅输出到文件（不输出到 stdout）

    // 日志轮转配置
    MaxSize    int  // 单文件最大 MB
    MaxBackups int  // 保留文件数
    MaxAge     int  // 保留天数
    Compress   bool // 压缩旧日志
}

// LoadFromEnv 从环境变量加载配置
func LoadFromEnv() *Config {
    cfg := &Config{
        Level:       getEnv("LOG_LEVEL", "info"),
        Format:      getEnv("LOG_FORMAT", "json"),
        ServiceName: getEnv("SERVICE_NAME", "emomo"),
        Environment: getEnv("APP_ENV", "local"),

        LogFile:     getEnv("LOG_FILE", "/var/log/emomo/app.log"),
        LogFileOnly: getEnvBool("LOG_FILE_ONLY", false),

        MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
        MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 7),
        MaxAge:     getEnvInt("LOG_MAX_AGE", 30),
        Compress:   getEnvBool("LOG_COMPRESS", true),
    }

    return cfg
}

// 辅助函数
func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    b, _ := strconv.ParseBool(val)
    return b
}

func getEnvInt(key string, defaultVal int) int {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    i, err := strconv.Atoi(val)
    if err != nil {
        return defaultVal
    }
    return i
}
```

#### 3.6 日志轮转支持

**依赖添加：**
```bash
go get gopkg.in/natefinch/lumberjack.v2
```

**文件：** `internal/logger/logger.go`（修改 New 函数）

```go
import (
    "io"
    "os"

    "github.com/sirupsen/logrus"
    "gopkg.in/natefinch/lumberjack.v2"
)

// New 创建日志器（支持环境变量配置）
func New(cfg *Config) *Logger {
    if cfg == nil {
        cfg = LoadFromEnv()
    }

    log := logrus.New()

    // 设置日志级别
    level, err := logrus.ParseLevel(cfg.Level)
    if err != nil {
        level = logrus.InfoLevel
    }
    log.SetLevel(level)

    // 设置输出格式
    if strings.ToLower(cfg.Format) == "text" {
        log.SetFormatter(&logrus.TextFormatter{
            FullTimestamp:   true,
            TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
        })
    } else {
        log.SetFormatter(&logrus.JSONFormatter{
            TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
            FieldMap: logrus.FieldMap{
                logrus.FieldKeyTime:  "timestamp",
                logrus.FieldKeyLevel: "level",
                logrus.FieldKeyMsg:   "message",
            },
            CallerPrettyfier: func(f *runtime.Frame) (string, string) {
                filename := path.Base(f.File)
                function := path.Base(f.Function)
                return function, fmt.Sprintf("%s:%d", filename, f.Line)
            },
        })
    }

    log.SetReportCaller(true)

    // 配置输出目标
    if cfg.Output != nil {
        // 优先使用显式指定的输出
        log.SetOutput(cfg.Output)
    } else {
        // 根据环境配置输出
        var writers []io.Writer

        // Stdout 输出
        if cfg.Environment == "local" || !cfg.LogFileOnly {
            writers = append(writers, os.Stdout)
        }

        // 文件输出（非 local 环境）
        if cfg.Environment != "local" && cfg.LogFile != "" {
            fileWriter := &lumberjack.Logger{
                Filename:   cfg.LogFile,
                MaxSize:    cfg.MaxSize,    // MB
                MaxBackups: cfg.MaxBackups,
                MaxAge:     cfg.MaxAge,     // days
                Compress:   cfg.Compress,
            }
            writers = append(writers, fileWriter)
        }

        if len(writers) == 0 {
            writers = append(writers, os.Stdout)
        }

        log.SetOutput(io.MultiWriter(writers...))
    }

    // 添加服务名字段
    entry := log.WithField("service", cfg.ServiceName)

    return &Logger{Entry: entry}
}

// NewDefault 创建使用环境变量配置的默认日志器
func NewDefault() *Logger {
    return New(nil)
}
```

#### 3.7 优雅关闭 (Graceful Shutdown)

**文件：** `internal/logger/logger.go`（扩展）

确保程序退出前所有日志被刷新到磁盘，避免日志丢失：

```go
import (
    "io"
    "sync"
)

// writerCloser 保存可关闭的 writer 引用
var (
    writerCloser   io.Closer
    writerCloserMu sync.Mutex
)

// Sync 刷新所有待写入的日志并关闭文件句柄
// 应在程序退出前调用，确保日志不丢失
//
// 使用示例：
//
//	func main() {
//	    // 初始化 logger
//	    logger.SetDefaultLogger(logger.NewDefault())
//	    defer logger.Sync() // 确保退出时刷新日志
//
//	    // ... 业务逻辑
//	}
//
// 或配合 signal 处理：
//
//	sigCh := make(chan os.Signal, 1)
//	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
//	<-sigCh
//	logger.Sync()
func Sync() error {
    writerCloserMu.Lock()
    defer writerCloserMu.Unlock()

    if writerCloser != nil {
        return writerCloser.Close()
    }
    return nil
}

// 在 New() 函数中保存 closer 引用（修改 New 函数）:
// 添加到文件输出配置部分:
//
//	if cfg.Environment != "local" && cfg.LogFile != "" {
//	    fileWriter := &lumberjack.Logger{...}
//	    writers = append(writers, fileWriter)
//
//	    // 保存 closer 引用以便 Sync() 使用
//	    writerCloserMu.Lock()
//	    writerCloser = fileWriter
//	    writerCloserMu.Unlock()
//	}
```

**集成到入口点：**

```go
// cmd/api/main.go
func main() {
    // 初始化
    appLogger := logger.NewDefault()
    logger.SetDefaultLogger(appLogger)

    // 确保退出时刷新日志
    defer func() {
        logger.Info("Shutting down, flushing logs...")
        if err := logger.Sync(); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
        }
    }()

    // 优雅关闭处理
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        logger.Info("Received shutdown signal")
        cancel()
    }()

    // ... 启动服务
}
```

---

### 阶段三：代码迁移 (Week 3-4)

#### 3.8 渐进式迁移策略

**迁移优先级：**

| 优先级 | 模块 | 原因 | 迁移工作量 |
|-------|------|------|-----------|
| **P0** | `cmd/api/main.go` | 入口点，影响全局日志器 | 小 |
| **P0** | `cmd/ingest/main.go` | 导入脚本内部 worker，影响全局日志器 | 小 |
| **P1** | `internal/api/middleware/logger.go` | 请求追踪，高价值 | 中 |
| **P1** | `internal/service/ingest.go` | 核心业务，日志最多（23 处） | 大 |
| **P2** | `internal/service/search.go` | 核心业务 | 中 |
| **P2** | `internal/api/handler/admin_handler.go` | 管理操作 | 中 |
| **P3** | `internal/repository/` | 数据库层 | 小 |
| **P3** | `internal/storage/` | 存储层 | 小 |
| **P3** | `internal/source/` | 数据源适配器 | 小 |

#### 3.9 迁移示例

**Before (现有代码):**
```go
// cmd/api/main.go
appLogger := logger.New(&logger.Config{
    Level:       "info",
    Format:      "json",
    ServiceName: "emomo-api",
})
```

**After (重构后):**
```go
// cmd/api/main.go
appLogger := logger.NewDefault() // 自动从环境变量加载配置
logger.SetDefault(appLogger)
```

---

**Before (现有代码):**
```go
// internal/api/middleware/logger.go
reqLogger := log.WithFields(logger.Fields{
    "request_id": uuid.New().String(),
    "path":       path,
    "method":     c.Request.Method,
    "client_ip":  c.ClientIP(),
})
ctx := reqLogger.WithContext(c.Request.Context())
c.Request = c.Request.WithContext(ctx)

// ... later
reqLogger.WithFields(logger.Fields{
    "status":     status,
    "latency_ms": latency.Milliseconds(),
    "size":       c.Writer.Size(),
}).Info("Request completed")
```

**After (重构后):**
```go
// internal/api/middleware/logger.go
ctx := c.Request.Context()

// 注入追踪字段（一次性）
ctx = logger.WithFields(ctx, logger.Fields{
    logger.FieldRequestID: uuid.New().String(),
    logger.FieldComponent: "api",
})
c.Request = c.Request.WithContext(ctx)

// 描述信息放 message
logger.CtxInfo(ctx, "Request started: method=%s, path=%s, client_ip=%s",
    c.Request.Method, path, c.ClientIP())

// ... later
// 指标字段用 Entry API
logger.With(logger.Fields{
    logger.FieldStatus:     status,
    logger.FieldDurationMs: latency.Milliseconds(),
    logger.FieldSize:       c.Writer.Size(),
}).Info(ctx, "Request completed: method=%s, path=%s", c.Request.Method, fullPath)
```

---

**Before (现有代码):**
```go
// internal/service/ingest.go
s.log(ctx).WithFields(logger.Fields{
    "source_id": result.sourceID,
}).WithError(result.err).Error("Failed to process item")
```

**After (重构后):**
```go
// internal/service/ingest.go (入口处注入追踪字段)
ctx = logger.WithFields(ctx, logger.Fields{
    logger.FieldComponent: "ingest",
    logger.FieldJobID:     uuid.New().String(),
    logger.FieldSource:    src.GetSourceID(),
})

// 错误日志
logger.CtxError(ctx, "Failed to process item: source_id=%s, error=%v", result.sourceID, result.err)
```

---

**Before (现有代码):**
```go
// internal/service/search.go
s.log(ctx).WithFields(logger.Fields{
    "original": req.Query,
    "expanded": expanded,
}).Info("Query expanded")
```

**After (重构后):**
```go
// internal/service/search.go (入口处注入追踪字段)
ctx = logger.SetComponent(ctx, "search")
ctx = logger.SetSearchID(ctx, uuid.New().String())

// 描述信息放 message
logger.CtxInfo(ctx, "Query expanded: original=%q, expanded=%q", req.Query, expanded)
```

---

### 阶段四：补充缺失日志 (Week 5)

#### 3.10 需要补充日志的模块

**A. 数据库层（`internal/repository/`）**

```go
// Before: 无日志
func (r *MemeRepository) Create(ctx context.Context, meme *domain.Meme) error {
    return r.db.Create(meme).Error
}

// After: 添加错误日志
func (r *MemeRepository) Create(ctx context.Context, meme *domain.Meme) error {
    err := r.db.Create(meme).Error
    if err != nil {
        logger.CtxError(ctx, "Failed to create meme: meme_id=%s, error=%v", meme.ID, err)
    }
    return err
}
```

**B. 存储层（`internal/storage/s3.go`）**

```go
// Before: 无日志
func (s *S3Storage) Upload(ctx context.Context, key string, data []byte) error {
    _, err := s.client.PutObject(ctx, &s3.PutObjectInput{...})
    return err
}

// After: 添加性能日志
func (s *S3Storage) Upload(ctx context.Context, key string, data []byte) error {
    startTime := time.Now()
    _, err := s.client.PutObject(ctx, &s3.PutObjectInput{...})
    duration := time.Since(startTime)

    if err != nil {
        logger.With(logger.Fields{
            logger.FieldDurationMs: duration.Milliseconds(),
            logger.FieldSize:       len(data),
        }).Error(ctx, "Failed to upload: key=%s, error=%v", key, err)
        return err
    }

    logger.With(logger.Fields{
        logger.FieldDurationMs: duration.Milliseconds(),
        logger.FieldSize:       len(data),
    }).Debug(ctx, "Upload completed: key=%s", key)
    return nil
}
```

**C. VLM 服务（`internal/service/vlm.go`）**

```go
// Before: 无详细日志
func (s *VLMService) GenerateDescription(ctx context.Context, imageURL string) (string, error) {
    // ... API 调用
}

// After: 添加性能和错误日志
func (s *VLMService) GenerateDescription(ctx context.Context, imageURL string) (string, error) {
    logger.CtxDebug(ctx, "Generating description: image_url=%s, model=%s", imageURL, s.model)

    startTime := time.Now()
    resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{...})
    duration := time.Since(startTime)

    if err != nil {
        logger.With(logger.Fields{
            logger.FieldDurationMs: duration.Milliseconds(),
        }).Error(ctx, "VLM API failed: image_url=%s, error=%v", imageURL, err)
        return "", err
    }

    description := resp.Choices[0].Message.Content
    logger.With(logger.Fields{
        logger.FieldDurationMs: duration.Milliseconds(),
    }).Info(ctx, "Description generated: length=%d", len(description))

    return description, nil
}
```

**D. Embedding 服务（`internal/service/embedding.go`）**

```go
// After: 添加性能和错误日志
func (s *JinaEmbedding) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
    startTime := time.Now()
    embedding, err := s.doAPICall(ctx, text)
    duration := time.Since(startTime)

    if err != nil {
        logger.With(logger.Fields{
            logger.FieldDurationMs: duration.Milliseconds(),
        }).Error(ctx, "Embedding API failed: error=%v", err)
        return nil, err
    }

    logger.With(logger.Fields{
        logger.FieldDurationMs: duration.Milliseconds(),
    }).Debug(ctx, "Embedding generated: dim=%d", len(embedding))

    return embedding, nil
}
```


## 4. 迁移前后对比

### 4.1 代码简洁度对比

| 场景 | 迁移前 | 迁移后 | 改进 |
|------|--------|--------|------|
| **Context 日志** | `s.log(ctx).Info("msg")` | `logger.CtxInfo(ctx, "msg")` | -30% 字符 |
| **带字段日志** | `s.log(ctx).WithFields(...).Info("msg")` | `logger.With(...).Info(ctx, "msg")` | 更清晰 |
| **注入追踪字段** | `logger.ContextWithFields(ctx, logger.Fields{"request_id": id})` | `logger.SetRequestID(ctx, id)` | -60% 字符 |
| **提取追踪字段** | 需手动从 context 提取 | `logger.GetRequestID(ctx)` | 新增功能 |

### 4.2 可观测性对比

| 维度 | 迁移前 | 迁移后 |
|------|--------|--------|
| **日志关联** | ❌ 缺少标准 `job_id`, `component` | ✅ 标准化追踪字段 |
| **性能监控** | ⚠️ `latency_ms` 不统一 | ✅ 统一 `duration_ms` 字段 |
| **日志查询** | ⚠️ Fields 和 message 混用 | ✅ 指标/描述清晰分离 |
| **配置灵活性** | ❌ 仅代码配置 | ✅ 环境变量 + 代码配置 |
| **日志轮转** | ❌ 无 | ✅ 自动轮转 + 压缩 |

### 4.3 实际业务场景示例

**场景：查询某个摄取任务的所有日志**

```bash
# 迁移前（困难）
grep "Starting ingestion" app.log | grep "source=localdir" # 只能找到开始日志
# 后续步骤的日志无法关联

# 迁移后（简单）
jq 'select(.job_id == "abc-123")' app.log # 自动关联所有相关日志
```

**场景：统计 VLM 调用失败**

```bash
# 迁移前（困难）
grep "VLM" app.log | grep "error" # 需要手动解析 message

# 迁移后（简单）
jq 'select(.level == "error" and .component == "vlm")' app.log | wc -l # 按组件过滤
```

**场景：查找响应时间超过 5 秒的请求**

```bash
# 迁移前（几乎不可能）
# latency_ms 字段存在，但没有统一位置

# 迁移后（简单）
jq 'select(.duration_ms > 5000)' app.log # 统一字段名
```

---

## 5. 参考资料

- [logrus 官方文档](https://github.com/sirupsen/logrus)
- [lumberjack 日志轮转库](https://github.com/natefinch/lumberjack)
- [12 Factor App - Logs](https://12factor.net/logs)
- Emomo 现有 Logger 实现 (`internal/logger/`)

---

## 附录 A：环境变量配置完整清单

```bash
# 基础配置
LOG_LEVEL=info              # debug, info, warn, error
LOG_FORMAT=json             # json, text
SERVICE_NAME=emomo          # 服务名称

# 环境配置
APP_ENV=local               # local, dev, prod

# 文件输出（非 local 环境生效）
LOG_FILE=/var/log/emomo/app.log
LOG_FILE_ONLY=false         # true 时仅输出到文件

# 日志轮转
LOG_MAX_SIZE=100            # 单文件最大 MB
LOG_MAX_BACKUPS=7           # 保留文件数
LOG_MAX_AGE=30              # 保留天数
LOG_COMPRESS=true           # 压缩旧日志
```
