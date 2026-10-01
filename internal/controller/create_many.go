package controller

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// batch is the body of a batch request: the items to create, update or
// patch, or the ids to delete.
//
// TODO: decide whether a batch update or patch may skip the items whose
// record does not exist and apply the rest, instead of failing whole. A
// switch for it would be a member of this body beside items, and the answer
// would have to list the items it skipped.
//
// TODO: decide whether a batch delete may accept an empty id instead of
// refusing the request, switched on by a member of this body beside ids.
type batch[M types.Model] struct {
	// IDs is the id list that should be batch delete.
	IDs []string `json:"ids,omitempty"`
	// Items is the resource list that should be batch create/update/partial
	// update. Each item is validated against its binding tags the way the
	// body of a single-resource request is: the validator only descends into
	// a slice told to dive. A batch patch validates each item on the fields
	// it names instead, see validatePatchFields.
	Items []M `json:"items,omitempty" binding:"dive"`
}

// repeatedID returns the error of a batch update or patch whose items name
// one record twice, items[1] names the record "r1", which items[0] already
// names, and nil when every item names a record of its own. The batch
// writes each record once, all of them or none, so a later item naming a
// record an earlier one names would write over it; the batch is refused
// before any record is read. An item naming no record is left to the
// flow, which refuses it.
func (req *batch[M]) repeatedID() error {
	named := make(map[string]int, len(req.Items))
	for i, m := range req.Items {
		id := m.GetID()
		if id == "" {
			continue
		}
		if first, ok := named[id]; ok {
			return errors.Newf("items[%d] names the record %q, which items[%d] already names", i, id, first)
		}
		named[id] = i
	}
	return nil
}

// CreateManyHandler returns a Gin handler that creates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch create flow (see createManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its CreateMany method runs on the bound payload.
func CreateManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.CreateMany, consts.CreateManyBefore, consts.CreateManyAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.CreateMany)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			response.Error(c, invalidArgument(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatch(&req)

		if err := a.createManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}

// CreateManyCall returns the batch create call of M on route, the
// counterpart of the handler CreateManyHandler returns for the generated
// handler of a CreateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch create flow (see createManyFlow)
// and answers with the items created, or with the status the failure maps
// to (see call).
func CreateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, consts.CreateMany, consts.CreateManyBefore, consts.CreateManyAfter)
	return func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return nil, c.invalid(err)
		}
		req := batch[M]{Items: items}
		normalizeBatch(&req)
		if err := validateRequest(&req); err != nil {
			return nil, c.invalidMessage(err)
		}
		if err := a.createManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}

// createManyFlow runs the batch create flow on the items of req: it takes the
// creator and updater of every item from the identity the request carries,
// runs the batch create hooks around the write, and records the operation.
// The items are req's own, filled by the write.
func (a *action[M, REQ, RSP]) createManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, consts.CreateMany)
	svc := a.service()
	val := a.newModel()
	username := requestctx.FromContext(ctx).Username()
	for _, m := range req.Items {
		m.SetCreatedBy(username)
		m.SetUpdatedBy(username)
	}

	// 1.Perform business logic processing before batch create resource.
	if err := a.traceServiceHook(ctx, consts.CreateManyBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.CreateManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 2.Batch create resource in database. Pure INSERT with one transaction
	// around the batch: any unique-key collision renders 409 and rolls the
	// whole batch back. A batch without items writes nothing.
	if err := database.Database[M](ctx).WithExpand(val.Expands()).Create(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch create resource
	if err := a.traceServiceHook(ctx, consts.CreateManyAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.CreateManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, val, consts.OP_CREATE_MANY,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
