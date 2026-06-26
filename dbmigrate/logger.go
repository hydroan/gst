package dbmigrate

import (
	"context"
	"time"

	"gorm.io/gorm/logger"
)

type dumperLogger struct {
	SQLs []string
}

func (l *dumperLogger) LogMode(level logger.LogLevel) logger.Interface     { return l }
func (l *dumperLogger) Info(ctx context.Context, msg string, data ...any)  {}
func (l *dumperLogger) Warn(ctx context.Context, msg string, data ...any)  {}
func (l *dumperLogger) Error(ctx context.Context, msg string, data ...any) {}

func (l *dumperLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.SQLs = append(l.SQLs, sql)
}
