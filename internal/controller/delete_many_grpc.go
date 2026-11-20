package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/types"
)

// DeleteManyCall returns the batch delete call of M on route, the
// counterpart of the handler DeleteManyHandler returns for the generated
// handler of a DeleteMany rpc: given the route parameters and the ids the
// request message carries, it validates the batch the way the handler
// validates a bound body, runs the batch delete flow (see deleteManyFlow)
// and answers nothing, or the status the failure maps to (see call).
func DeleteManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, ids []string) error {
	a := newAction[M, M, M](route, apiaction.DeleteMany, apiaction.DeleteManyBefore, apiaction.DeleteManyAfter)
	return func(ctx context.Context, params map[string]string, ids []string) error {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return c.invalid(err)
		}
		req := batch[M]{IDs: ids}
		normalizeBatch(&req)
		if err := validateRequest(&req); err != nil {
			return c.invalidMessage(err)
		}
		if err := a.deleteManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return c.fail(err)
		}
		return c.finish()
	}
}
