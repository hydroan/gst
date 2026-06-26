package zap

import (
	"context"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	gorml "gorm.io/gorm/logger"
)

// GormLogger implements gorm logger.Interface
type GormLogger struct{ l types.Logger }

var _ gorml.Interface = (*GormLogger)(nil)

func (g *GormLogger) LogMode(gorml.LogLevel) gorml.Interface           { return g }
func (g *GormLogger) Info(_ context.Context, str string, args ...any)  { g.l.Infow(str, args) }
func (g *GormLogger) Warn(_ context.Context, str string, args ...any)  { g.l.Warnw(str, args) }
func (g *GormLogger) Error(_ context.Context, str string, args ...any) { g.l.Errorw(str, args) }
func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	meta := requestctx.FromContext(ctx)
	username := meta.Username()
	userID := meta.UserID()
	traceID := meta.TraceID()
	// Fallback to OTEL span context trace ID when request metadata has no trace ID.
	if len(traceID) == 0 {
		spanCtx := trace.SpanFromContext(ctx).SpanContext()
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}
	}
	elapsed := time.Since(begin)
	sql, rows := fc()

	if err != nil {
		g.l.Errorz("", zap.String("sql", sql), zap.Int64("rows", rows), zap.String("elapsed", util.FormatDurationSmart(elapsed)), zap.Error(err))
	} else {
		if elapsed > config.App.Database.SlowQueryThreshold {
			g.l.Warnz("slow SQL detected",
				zap.String(consts.CTX_USERNAME, username),
				zap.String(consts.CTX_USER_ID, userID),
				zap.String(consts.TRACE_ID, traceID),
				zap.String("sql", sql),
				zap.String("elapsed", util.FormatDurationSmart(elapsed)),
				zap.String("threshold", config.App.Database.SlowQueryThreshold.String()),
				zap.Int64("rows", rows))
		} else {
			g.l.Infoz("sql executed",
				zap.String(consts.CTX_USERNAME, username),
				zap.String(consts.CTX_USER_ID, userID),
				zap.String(consts.TRACE_ID, traceID),
				zap.String("sql", sql),
				zap.String("elapsed", util.FormatDurationSmart(elapsed)),
				zap.Int64("rows", rows))
		}
	}
}
