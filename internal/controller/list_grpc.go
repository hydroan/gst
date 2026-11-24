package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
)

// ListCall returns the list call of M on route, the counterpart of the
// handler ListHandler returns for the generated handler of a List rpc: given
// the route parameters and the query the request message carries, it runs
// the list flow (see listFlow) on the query rendered as the HTTP one (see
// Query) and answers with the items, the total and whether records lie past
// the page, or with the status the failure maps to (see call).
func ListCall[M gst.Model](route string) func(ctx context.Context, params map[string]string, query Query) ([]M, int, bool, error) {
	a := newAction[M, M, M](route, apiaction.List, apiaction.ListBefore, apiaction.ListAfter)
	return func(ctx context.Context, params map[string]string, query Query) ([]M, int, bool, error) {
		c, err := a.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return nil, 0, false, c.invalid(err)
		}
		items, total, hasMore, err := a.listFlow(c.ctx, c.serviceContext)
		if err != nil {
			return nil, 0, false, c.fail(err)
		}
		if err := c.finish(); err != nil {
			return nil, 0, false, err
		}
		return items, total, hasMore, nil
	}
}
