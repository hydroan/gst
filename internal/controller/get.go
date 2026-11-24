package controller

import (
	"context"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// getFlow runs the get flow on the record id names: it applies the expansion
// and depth query options the request carries, runs the get hooks around the
// read, records the operation, and returns the model. id must not be empty:
// a UUID-keyed model mints a fresh id for an empty one (see setID). An
// id the model rejects, and a read that finds no stored record, both answer
// 404 (see notFound); an expansion or a depth the request spells wrong
// answers 400 (see parseExpandQuery).
func (a *action[M, REQ, RSP]) getFlow(ctx context.Context, newServiceContext serviceContextFunc, id string) (M, error) {
	var zero M
	log := logger.Controller.WithContext(ctx, apiaction.Get)
	svc := a.service()

	// The GetBefore hook reads the id, so it is set first.
	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 before
		// the raw value reaches SQL, where implicit string-to-integer
		// coercion could match an unintended row.
		return zero, reportFailure(log, trace.SpanFromContext(ctx), "route id rejected by model", nil, notFound(nil), zap.String("id", id))
	}
	expands, err := parseExpandQuery(requestctx.QueryValues(ctx), m)
	if err != nil {
		return zero, failWith(ctx, log, "parse query parameter failed", err, invalidArgument(err))
	}

	// 1.Perform business logic processing before get resource.
	if err := a.traceServiceHook(ctx, apiaction.GetBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.GetBefore(sc, m)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Get resource from database. The database layer answers existence:
	// database.ErrRecordNotFound renders 404 instead of the server's own
	// failure.
	if err := database.Database[M](ctx).WithExpand(expands).Get(m, m.GetID()); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after get resource.
	if err := a.traceServiceHook(ctx, apiaction.GetAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.GetAfter(sc, m)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// A model without an id or creation time holds no stored record (a
	// missing row already failed above with ErrRecordNotFound), so answer
	// 404 instead of an empty resource.
	if len(m.GetID()) == 0 || m.GetCreatedAt().Equal(time.Time{}) {
		return zero, reportFailure(log, trace.SpanFromContext(ctx), notFoundMsg, nil, notFound(nil))
	}

	// 4.record operation log to database.
	if err := audit.RecordOperation(ctx, m, gst.OperationGet,
		func() *modellogmgmt.OperationLog {
			return operationLog(ctx, a.name)
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return m, nil
}
