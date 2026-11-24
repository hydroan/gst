package controller

import (
	"context"
	"reflect"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
)

// CreateCall returns the create call of M on route, the counterpart of the
// handler CreateHandler returns for the generated handler of a Create rpc:
// given the route parameters and the model the request message decoded
// into, it validates the model the way the handler validates a bound body,
// runs the create flow (see createFlow) and answers with the model created,
// or with the status the failure maps to (see call).
func CreateCall[M gst.Model](route string) func(ctx context.Context, params map[string]string, m M) (M, error) {
	a := newAction[M, M, M](route, apiaction.Create, apiaction.CreateBefore, apiaction.CreateAfter)
	return func(ctx context.Context, params map[string]string, m M) (M, error) {
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
		if err := a.createFlow(c.ctx, c.serviceContext, m); err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}
