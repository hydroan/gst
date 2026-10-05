package controller

import (
	"context"
	"encoding/json"
	"io"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// PatchHandler returns a Gin handler that partially updates one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (the id carried by the body is ignored),
// binds the JSON body into M along with the set of fields the body carried,
// and runs the patch flow (see patchFlow), which answers with the patched
// record.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Patch method runs on the bound payload.
func PatchHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Patch, consts.PatchBefore, consts.PatchAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		var id string

		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.Patch)

		req := a.newModel()
		body, err := readJSONRequestBody(c)
		if err != nil {
			log.Errorz("bind request body failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}
		fields, err := patchFieldSetFromJSONBody(a.typ, body)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}
		// A versioned model must carry the version it was read with on every
		// partial update, exactly like Update: the merge would otherwise keep
		// the freshly loaded row's version and the lock would silently check
		// the row against itself. Enforced before the existence query so a
		// defective request costs no database work. See modelregistry.Version.
		if versionField, versioned := modelregistry.VersionFieldName(req); versioned {
			if _, ok := fields[versionField]; !ok {
				log.Errorz("versioned model patched without its version", zap.String("field", versionField))
				response.Error(c, databaseError(database.ErrVersionRequired))
				gstotel.RecordError(span, database.ErrVersionRequired)
				return
			}
		}
		// The resource id comes from the configured route parameter only.
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if err = decodeJSONRequest(c, &req); err != nil {
			// A single-resource patch without a body patches nothing; refuse it
			// with a stable message instead of the bare io.EOF text.
			err = requiredBodyError(err)
			log.Errorz("bind request body failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}
		if err = validatePatchFields(req, fields); err != nil {
			err = clientSafeBindError(err)
			log.Errorz("bind request body failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}
		a.normalizeModel(&req)
		if len(id) == 0 {
			log.Errorz(missingRouteParamMsg)
			response.Error(c, badRequest(missingRouteParamMsg))
			gstotel.RecordError(span, errors.New(missingRouteParamMsg))
			return
		}

		cur, err := a.patchFlow(requestContext(c), ginServiceContext(c), id, req, fields)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, cur)
	}
}

// PatchCall returns the patch call of M on route, the counterpart of the
// handler PatchHandler returns for the generated handler of a Patch rpc:
// given the route parameters, the id the request message names the record
// by, the values it decoded into and the paths of its update mask, which
// name the fields to apply as the message names them (see maskFieldSet), it
// refuses a message carrying no record the way the handler refuses a
// request without a body, validates the fields the mask names the way the
// handler validates the fields a body names (see validatePatchFields),
// refuses a versioned model patched without its version the way the
// handler does, runs the patch flow (see patchFlow) and answers with the
// record patched, or with the status the failure maps to (see call).
func PatchCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M, paths []string) (M, error) {
	a := newAction[M, M, M](route, consts.Patch, consts.PatchBefore, consts.PatchAfter)
	return func(ctx context.Context, params map[string]string, id string, m M, paths []string) (M, error) {
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
		fields, err := maskFieldSet(a.typ, paths)
		if err != nil {
			return zero, c.invalid(err)
		}
		if versionField, versioned := modelregistry.VersionFieldName(m); versioned {
			if _, ok := fields[versionField]; !ok {
				return zero, c.refuse(databaseError(database.ErrVersionRequired), errors.Wrapf(database.ErrVersionRequired, "patch %s without its %s", a.name, versionField))
			}
		}
		if err = validatePatchFields(m, fields); err != nil {
			return zero, c.invalidMessage(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		cur, err := a.patchFlow(c.ctx, c.serviceContext, id, m, fields)
		if err != nil {
			return zero, c.fail(err)
		}
		return answer(c, cur)
	}
}

// patchFlow runs the patch flow on the record id names: it loads the record,
// copies the fields of req present in the request (fields) into it, sets the
// updater from the identity the request carries, runs the patch hooks around
// the write, records the operation, and returns the patched record. id must
// not be empty (see setID); an id the model rejects, and one naming no
// record, both answer 404 (see notFound).
//
// The write is the whole record loaded, not only the fields the request
// carried. Concurrent patches of one record therefore resolve as last writer
// wins: a later patch puts back the fields an earlier one changed, even when
// the two touched different fields, and both answer success. That is the
// default contract of every framework update, not a defect; a model that
// needs the stale write refused instead declares model.Version.
func (a *action[M, REQ, RSP]) patchFlow(ctx context.Context, newServiceContext serviceContextFunc, id string, req M, fields patchFieldSet) (M, error) {
	var zero M
	log := logger.Controller.WithContext(ctx, consts.Patch)
	svc := a.service()

	data := make([]M, 0)
	// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 without
		// relying on the empty-query safety net below.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return zero, notFound(nil)
	}

	// Make sure the record already exists. The read is pinned to
	// the primary because what it reads is written straight back: the
	// patch merges onto this row, so a replica still catching up would
	// have the fields the request does not touch written back stale.
	if err := database.Database[M](ctx).WithReplica(false).WithLimit(1).WithQuery(m).List(&data); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	if len(data) != 1 {
		log.Errorz("records matched by id is not exactly one", zap.Int("count", len(data)), zap.String("id", id))
		return zero, notFound(nil)
	}
	data[0].SetUpdatedBy(requestctx.FromContext(ctx).Username())

	newVal := reflect.ValueOf(req).Elem()
	oldVal := reflect.ValueOf(data[0]).Elem()
	applyPatch(log, a.typ, oldVal, newVal, fields)
	cur := oldVal.Addr().Interface().(M) //nolint:errcheck

	// 1.Perform business logic processing before partial update resource.
	if err := a.traceServiceHook(ctx, consts.PatchBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchBefore(sc, cur)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Partial update resource in database. The record was loaded above, so
	// ErrRecordNotFound only fires when it vanished in between; unique-key
	// collisions from the patched values render 409.
	if err := database.Database[M](ctx).Update(cur); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after partial update resource.
	if err := a.traceServiceHook(ctx, consts.PatchAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchAfter(sc, cur)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// NOTE: We should record the `req` instead of `oldVal`, the req is `newVal`.
	// Record and Request both carry the request payload, so one marshal
	// feeds both columns; Response carries the resulting row instead. The
	// entry names the patched record by its own id: the body need not carry
	// one, the route named the record.
	if err := audit.RecordOperation(ctx, req, consts.OP_PATCH,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			respData, _ := json.Marshal(cur)
			entry := operationLog(ctx, a.name)
			entry.RecordID = cur.GetID()
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(respData)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return cur, nil
}
