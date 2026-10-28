package controller

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
)

// UpdateHandler returns a Gin handler that replaces one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, reads the resource id from the configured route parameter (the id carried
// by the body is ignored), and runs the update flow (see updateFlow), which
// answers with the replacement.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Update method runs on the bound payload.
func UpdateHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Update, consts.UpdateBefore, consts.UpdateAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), consts.Update)

		req := a.newModel()
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// A full update replaces the resource, so an absent body is refused
			// rather than tolerated as "nothing to change".
			reqErr = requiredBodyError(reqErr)
			log.Errorz("bind request body failed", zap.Error(reqErr))
			response.Error(c, invalidArgument(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}

		id, ok := routeID(c, cfg, log, span)
		if !ok {
			return
		}

		if err := a.updateFlow(requestContext(c), ginServiceContext(c), id, req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}

// UpdateCall returns the update call of M on route, the counterpart of the
// handler UpdateHandler returns for the generated handler of an Update rpc:
// given the route parameters, the id the request message names the record
// by and the replacement it decoded into, it validates the replacement the
// way the handler validates a bound body, runs the update flow (see
// updateFlow) and answers with the replacement as stored, or with the
// status the failure maps to (see call).
func UpdateCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
	a := newAction[M, M, M](route, consts.Update, consts.UpdateBefore, consts.UpdateAfter)
	return func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
		var zero M
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		if reflect.ValueOf(m).IsNil() {
			return zero, c.missingRecord()
		}
		a.normalizeModel(&m)
		if err := validateRequest(m); err != nil {
			return zero, c.invalidMessage(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		if err := a.updateFlow(c.ctx, c.serviceContext, id, m); err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}

// updateFlow runs the update flow, replacing the record id names with req: it
// clears the creator and updater req carries (see clearAudit), runs the
// update hooks around the write, which names the caller the request carries
// as the updater, and records the operation. Existence is enforced by
// the database layer instead of a pre-read: a missing or soft-deleted record
// surfaces as database.ErrRecordNotFound and renders 404, and a unique-key
// collision renders 409. The UpdateBefore service hook therefore runs before
// existence is known. After a successful write, and before the hook after
// it, the flow reads the record back from the primary and backfills the
// columns an update leaves alone, the creation audit among them (see
// backfillUnwrittenColumns), keeping the rest of req intact so
// hook-populated fields survive; req is the replacement answered. The id req
// carries is replaced by id, which must not be empty (see setID): SetID
// keeps an id already set, so the one the body carries is cleared first, or
// the replacement would land on the record it names; an id the model rejects
// answers 404 (see notFound) without touching the database.
func (a *action[M, REQ, RSP]) updateFlow(ctx context.Context, newServiceContext serviceContextFunc, id string, req M) error {
	log := logger.Controller.WithContext(ctx, consts.Update)
	svc := a.service()

	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 without
		// touching the database.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return notFound(nil)
	}
	req.ClearID()
	req.SetID(id)
	clearAudit(req)

	// 1.Perform business logic processing before update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.UpdateBefore(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Update resource in database. The database layer answers existence:
	// ErrRecordNotFound renders 404, ErrDuplicatedKey renders 409.
	if err := database.Database[M](ctx).Update(req); err != nil {
		return failDatabase(ctx, log, err)
	}
	// The columns the update left alone hold whatever the client sent; they
	// are read back as stored, from the primary, before the hook after the
	// write sees them (see backfillUnwrittenColumns). On a reload failure
	// keep req as is: the update itself already committed.
	reloaded := a.newModel()
	if reloadErr := database.Database[M](ctx).WithReplica(false).Get(reloaded, id); reloadErr != nil {
		log.Warnz("reload the columns the update leaves alone failed", zap.Error(reloadErr))
	} else {
		backfillUnwrittenColumns(a.typ, reflect.ValueOf(req).Elem(), reflect.ValueOf(reloaded).Elem())
	}
	// 3.Perform business logic processing after update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.UpdateAfter(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, req, consts.OP_UPDATE,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.RecordID = req.GetID()
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}

// backfillUnwrittenColumns copies onto dst, from stored, the columns an
// update leaves alone, those gorm writes on insert only or never (see
// modelschema.Column.Updatable), the creation audit among them: dst holds
// what the request carried for them, which the database did not take, and an
// update answers what is stored. The columns an update writes keep dst's
// values, so what a hook set on them stands, and so do the fields no column
// stores.
func backfillUnwrittenColumns(typ reflect.Type, dst, stored reflect.Value) {
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return
	}
	for _, col := range columns {
		if col.Updatable {
			continue
		}
		value, err := stored.FieldByIndexErr(col.Index)
		if err != nil {
			continue
		}
		if target := fieldByIndexAllocating(dst, col.Index); target.IsValid() && target.CanSet() {
			target.Set(value)
		}
	}
}
