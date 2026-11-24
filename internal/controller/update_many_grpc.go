package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
)

// UpdateManyCall returns the batch update call of M on route, the
// counterpart of the handler UpdateManyHandler returns for the generated
// handler of an UpdateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch update flow (see updateManyFlow)
// and answers with the items as stored, or with the status the failure maps
// to (see call).
func UpdateManyCall[M gst.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter)
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
		if err := a.updateManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}
