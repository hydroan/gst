package controller

import (
	"context"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/types"
)

// GetCall returns the get call of M on route, the counterpart of the handler
// GetHandler returns for the generated handler of a Get rpc: given the route
// parameters, the id the request message names the record by and its
// expansion query, it runs the get flow (see getFlow) and answers with the
// model, or with the status the failure maps to (see call); a message
// naming no record is refused the way a request missing its route parameter
// is.
func GetCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, query Query) (M, error) {
	a := newAction[M, M, M](route, apiaction.Get, apiaction.GetBefore, apiaction.GetAfter)
	return func(ctx context.Context, params map[string]string, id string, query Query) (M, error) {
		var zero M
		c, err := a.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		m, err := a.getFlow(c.ctx, c.serviceContext, id)
		if err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}
