package controller

import (
	"context"
	"encoding/json"
	"io"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
)

// PatchManyHandler returns a Gin handler that partially updates multiple
// resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M] along with the set of fields each item carried, and runs the
// batch patch flow (see patchManyFlow), which answers with the patched
// records.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its PatchMany method runs on the bound payload.
func PatchManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.PatchMany)

		var req batch[M]
		body, err := readJSONRequestBody(c)
		if err != nil {
			log.Errorz("bind request body failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}
		fieldSets, fieldErr := patchManyFieldSetsFromJSONBody(a.typ, body)
		if fieldErr != nil && !errors.Is(fieldErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(fieldErr))
			response.Error(c, invalidArgument(fieldErr))
			gstotel.RecordError(span, fieldErr)
			return
		}
		if reqErr := decodeJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			response.Error(c, invalidArgument(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatch(&req)
		// A versioned model must carry a version on every item, exactly like
		// the single-resource patch; failing the whole batch up front keeps
		// the all-or-nothing shape a defective request deserves. See
		// modelschema.Version. Each item is then validated on the fields it
		// names, like the single-resource patch too. Every version is checked
		// before any item is validated, the order the call checks a batch in,
		// so a batch defective in several items is refused for the same one
		// on both transports.
		versionField, versioned := modelschema.VersionFieldName(a.newModel())
		fieldsOf := func(i int) patchFieldSet {
			if i < len(fieldSets) {
				return fieldSets[i]
			}
			return patchFieldSet{}
		}
		for i := range req.Items {
			if _, ok := fieldsOf(i)[versionField]; versioned && !ok {
				log.Errorz("versioned model patched without its version",
					zap.Int("item", i), zap.String("field", versionField))
				response.Error(c, databaseError(database.ErrVersionRequired))
				gstotel.RecordError(span, database.ErrVersionRequired)
				return
			}
		}
		for i, item := range req.Items {
			if fieldErr := validatePatchFields(item, fieldsOf(i)); fieldErr != nil {
				fieldErr = clientSafeItemBindError(i, fieldErr)
				log.Errorz("bind request body failed", zap.Error(fieldErr))
				response.Error(c, invalidArgument(fieldErr))
				gstotel.RecordError(span, fieldErr)
				return
			}
		}

		rsp, err := a.patchManyFlow(requestContext(c), ginServiceContext(c), &req, fieldSets)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, rsp)
	}
}

// PatchManyCall returns the batch patch call of M on route, the counterpart
// of the handler PatchManyHandler returns for the generated handler of a
// PatchMany rpc: given the route parameters, the items the request message
// decoded into and the paths of each item's update mask, one mask per item
// in order (see maskFieldSet), it validates each item on the fields its mask
// names the way the handler validates the fields each item names (see
// validatePatchFields), refuses a versioned model whose item carries no
// version the way the handler does, runs the batch patch flow (see
// patchManyFlow) and answers with the records patched, or with the status
// the failure maps to (see call).
func PatchManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M, paths [][]string) ([]M, error) {
	a := newAction[M, M, M](route, apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
	return func(ctx context.Context, params map[string]string, items []M, paths [][]string) ([]M, error) {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return nil, c.invalid(err)
		}
		for i, item := range items {
			if reflect.ValueOf(item).IsNil() {
				return nil, c.invalid(errors.Newf("items[%d] carries no record", i))
			}
		}
		req := batch[M]{Items: items}
		normalizeBatch(&req)
		if len(paths) != len(req.Items) {
			return nil, c.invalid(errors.Newf("%d items carry %d update masks; each item names the fields to apply in a mask of its own", len(req.Items), len(paths)))
		}
		fieldSets := make([]patchFieldSet, len(paths))
		for i, itemPaths := range paths {
			fields, maskErr := maskFieldSet(a.typ, itemPaths)
			if maskErr != nil {
				return nil, c.invalid(errors.Wrapf(maskErr, "items[%d]", i))
			}
			fieldSets[i] = fields
		}
		if versionField, versioned := modelschema.VersionFieldName(a.newModel()); versioned {
			for i, itemFields := range fieldSets {
				if _, ok := itemFields[versionField]; !ok {
					return nil, c.refuse(databaseError(database.ErrVersionRequired), errors.Wrapf(database.ErrVersionRequired, "patch many %s item %d without its %s", a.name, i, versionField))
				}
			}
		}
		for i, item := range req.Items {
			if err = validatePatchFields(item, fieldSets[i]); err != nil {
				return nil, c.invalidItemMessage(i, err)
			}
		}
		rsp, err := a.patchManyFlow(c.ctx, c.serviceContext, &req, fieldSets)
		if err != nil {
			return nil, c.fail(err)
		}
		return answer(c, rsp.Items)
	}
}

// patchManyFlow runs the batch patch flow on the items of req: it loads the
// record each item names, copies the fields the item carried (fieldSets, one
// set per item in order; an item past its end carries none) into that
// record, clears the updater it was stored with, runs the batch patch hooks
// around the write, which names the caller the request carries as the
// updater, records the operation, and returns the batch with the patched
// records, as the hooks left them, in place of the items. The batch patches
// all of its items or none, as a batch update does: a batch naming one
// record twice (see repeatedID) and an item without an id fail the whole
// batch with 400 before any record is read, and an item whose record does
// not exist fails it with 404 before anything is written.
//
// Each write is the whole record loaded, not only the fields its item
// carried, so concurrent patches of one record resolve as last writer wins
// exactly as in patchFlow: a later patch puts back the fields an earlier one
// changed, even different ones, and both answer success. A model that needs
// the stale write refused instead declares model.Version.
func (a *action[M, REQ, RSP]) patchManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M], fieldSets []patchFieldSet) (batch[M], error) {
	var zero batch[M]
	log := logger.Controller.WithContext(ctx, apiaction.PatchMany)
	svc := a.service()

	if err := req.repeatedID(); err != nil {
		return zero, failWith(ctx, log, "batch patch naming a record twice", err, invalidArgument(err))
	}
	// An item without an id names no record: a defective request, refused
	// before any record is read with the sentence PatchItem refuses it with
	// for the generated handler (see itemWithoutID). Setting an empty id on
	// a UUID-keyed model would mint a fresh one instead.
	for i, m := range req.Items {
		if len(m.GetID()) == 0 {
			err := errors.Wrapf(database.ErrIDRequired, "patch many %s item %d", a.name, i)
			return zero, failWith(ctx, log, "batch patch item without its id", err, itemWithoutID(i))
		}
	}
	// One statement reads the records of the batch, pinned to the primary:
	// each row read here is merged with its patch and written straight back,
	// so a stale one would write the untouched fields back as they were on
	// the replica.
	stored, err := a.recordsByID(database.Database[M](ctx).WithReplica(false), req.itemIDs())
	if err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	patched := make([]M, 0, len(req.Items))
	for i, m := range req.Items {
		current, ok := stored[m.GetID()]
		if !ok {
			err := errors.Wrapf(database.ErrRecordNotFound, "patch many %s id=%s", a.name, m.GetID())
			return zero, failWith(ctx, log, "partial update resource not found", err, databaseError(err))
		}
		fields := patchFieldSet{}
		if i < len(fieldSets) {
			fields = fieldSets[i]
		}
		applyPatch(log, a.typ, reflect.ValueOf(current).Elem(), reflect.ValueOf(m).Elem(), fields)
		// The updater is the write's to name, as in patchFlow.
		current.SetUpdatedBy("")
		patched = append(patched, current)
	}

	// 1.Perform business logic processing before batch patch resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchManyBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchManyBefore(sc, patched...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Batch partial update resource in database. The rows were loaded
	// above, so ErrRecordNotFound only fires when one vanished in between;
	// unique-key collisions from the patched values render 409. Either way
	// the transaction rolls the whole batch back. A batch without items
	// writes nothing.
	if err := database.Database[M](ctx).Update(patched...); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch patch resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchManyAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchManyAfter(sc, patched...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}

	// The response carries the patched records, whose fields the hooks
	// may have set, not the items of the request.
	rsp := *req
	rsp.Items = patched

	// 4.record operation log to database.
	// Record and Request both carry the request payload, so one marshal
	// feeds both columns; Response carries the patched records instead.
	if err := audit.RecordOperation(ctx, a.newModel(), types.OperationPatchMany,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			respData, _ := json.Marshal(rsp)
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(respData)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return rsp, nil
}
