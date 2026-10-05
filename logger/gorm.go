package logger

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dbruntime/dbnode"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"
)

// GormLogger implements gorm logger.Interface
type GormLogger struct{ l types.Logger }

var _ gormlogger.Interface = (*GormLogger)(nil)

func (g *GormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return g }

// Info, Warn and Error carry gorm's own non-statement messages (migrator
// progress, callback registration problems). gorm passes a printf format
// string with its arguments, so they forward to the formatting loggers.
func (g *GormLogger) Info(_ context.Context, str string, args ...any)  { g.l.Infof(str, args...) }
func (g *GormLogger) Warn(_ context.Context, str string, args ...any)  { g.l.Warnf(str, args...) }
func (g *GormLogger) Error(_ context.Context, str string, args ...any) { g.l.Errorf(str, args...) }

// traceFieldCap is the most fields one Trace entry carries: caller, the
// eight base fields, cronjob or leader (an identity carries at most one of
// them), db_role, record_not_found, and the one field the error/slow branches
// append (mutually exclusive in the switch). Trace sizes its field slice to
// it once, so the hot path never regrows; a field added to Trace bumps it,
// which the worst-case test enforces.
const traceFieldCap = 13

// Trace logs one executed statement with the request identity, timing, the
// SQL text, and the business caller that issued it.
//
// gorm.ErrRecordNotFound stays at info level with a record_not_found marker:
// a read matching no row is a documented outcome of First/Take, and logging
// it as an error buries real failures under normal traffic. A statement its
// context canceled logs at info level as canceled, for the same reason:
// whoever canceled it — a client gone, a shutdown, a holder that stopped
// renewing its lease — ended it, and nothing failed in the database; a
// statement past its deadline is a failure, the database having taken too
// long. Statements over the configured threshold log as slow queries; real
// errors log at error level with the same field set.
func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	meta := requestctx.FromContext(ctx)
	username := meta.Username()
	userID := meta.UserID()
	id := execctx.FromContext(ctx)
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := make([]zap.Field, 0, traceFieldCap)
	if caller, ok := callerOutside(isSkippedSQLFrame); ok {
		fields = append(fields, zap.String("caller", caller))
	}
	fields = append(
		fields,
		zap.String(consts.CTX_ROUTE, meta.Route()),
		zap.String(consts.CTX_METHOD, meta.Method()),
		zap.String(consts.CTX_USERNAME, username),
		zap.String(consts.CTX_USER_ID, userID),
		zap.String(consts.TRACE_ID, id.TraceID),
		zap.String("sql", sql),
		util.LogDuration(elapsed),
		zap.Int64("rows", rows),
	)
	// Present only inside a cron round or a leader tenure; a request's lines
	// carry no empty field for a capability they do not use.
	if len(id.Cronjob) > 0 {
		fields = append(fields, zap.String(consts.CRONJOB, id.Cronjob))
	}
	if len(id.Leader) > 0 {
		fields = append(fields, zap.String(consts.LEADER, id.Leader))
	}
	// Present only on handles with read replicas attached, where "which node
	// served this query" stops being answerable by assumption.
	if role := dbnode.RoleFromContext(ctx); len(role) > 0 {
		fields = append(fields, zap.String("db_role", role))
	}
	notFound := errors.Is(err, gormlogger.ErrRecordNotFound)
	if notFound {
		fields = append(fields, zap.Bool("record_not_found", true))
	}

	switch {
	case errors.Is(err, context.Canceled):
		g.l.Infoz("sql canceled", fields...)
	case err != nil && !notFound:
		g.l.Errorz("sql failed", append(fields, zap.Error(err))...)
	case elapsed > config.App.Database.SlowQueryThreshold:
		g.l.Warnz("slow sql detected", append(fields, zap.String("threshold", config.App.Database.SlowQueryThreshold.String()))...)
	default:
		g.l.Infoz("sql executed", fields...)
	}
}
