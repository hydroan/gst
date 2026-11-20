package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/types"
)

// CreateManyCall returns the batch create call of M on route, the
// counterpart of the handler CreateManyHandler returns for the generated
// handler of a CreateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch create flow (see createManyFlow)
// and answers with the items created, or with the status the failure maps
// to (see call).
func CreateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, apiaction.CreateMany, apiaction.CreateManyBefore, apiaction.CreateManyAfter)
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
