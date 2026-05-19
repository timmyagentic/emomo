package repository

import (
	"context"
	"errors"
	"time"

	"github.com/timmy/emomo/internal/logger"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type structuredGormLogger struct {
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(level gormlogger.LogLevel) gormlogger.Interface {
	return &structuredGormLogger{
		level:         level,
		slowThreshold: 200 * time.Millisecond,
	}
}

func (l *structuredGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	next := *l
	next.level = level
	return &next
}

func (l *structuredGormLogger) Info(ctx context.Context, format string, args ...interface{}) {
	if l.level >= gormlogger.Info {
		logger.With(logger.Fields{logger.FieldComponent: "db"}).Info(ctx, format, args...)
	}
}

func (l *structuredGormLogger) Warn(ctx context.Context, format string, args ...interface{}) {
	if l.level >= gormlogger.Warn {
		logger.With(logger.Fields{logger.FieldComponent: "db"}).Warn(ctx, format, args...)
	}
}

func (l *structuredGormLogger) Error(ctx context.Context, format string, args ...interface{}) {
	if l.level >= gormlogger.Error {
		logger.With(logger.Fields{logger.FieldComponent: "db"}).Error(ctx, format, args...)
	}
}

func (l *structuredGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.level >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		logger.With(gormTraceFields(elapsed, rows)).Error(ctx, "GORM query failed: sql=%s, error=%v", sql, err)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		sql, rows := fc()
		logger.With(gormTraceFields(elapsed, rows)).Warn(ctx, "GORM slow query: sql=%s", sql)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		logger.With(gormTraceFields(elapsed, rows)).Info(ctx, "GORM query: sql=%s", sql)
	}
}

func gormTraceFields(elapsed time.Duration, rows int64) logger.Fields {
	return logger.Fields{
		logger.FieldComponent:  "db",
		logger.FieldDurationMs: elapsed.Milliseconds(),
		"rows":                 rows,
	}
}
