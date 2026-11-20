package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/types"
)

// DeleteCall returns the delete call of M on route, the counterpart of the
// handler DeleteHandler returns for the generated handler of a Delete rpc:
// given the route parameters and the id the request message names the
// record by, it runs the delete flow (see deleteFlow) and answers nothing,
// or the status the failure maps to (see call).
func DeleteCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string) error {
	a := newAction[M, M, M](route, apiaction.Delete, apiaction.DeleteBefore, apiaction.DeleteAfter)
	return func(ctx context.Context, params map[string]string, id string) error {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return c.invalid(err)
		}
		if id == "" {
			return c.missingID()
		}
		if err := a.deleteFlow(c.ctx, c.serviceContext, id); err != nil {
			return c.fail(err)
		}
		return c.finish()
	}
}
