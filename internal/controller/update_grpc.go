package controller

import (
	"context"
	"reflect"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
)

// UpdateCall returns the update call of M on route, the counterpart of the
// handler UpdateHandler returns for the generated handler of an Update rpc:
// given the route parameters, the id the request message names the record
// by and the replacement it decoded into, it validates the replacement the
// way the handler validates a bound body, runs the update flow (see
// updateFlow) and answers with the replacement as stored, or with the
// status the failure maps to (see call).
func UpdateCall[M gst.Model](route string) func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
	a := newAction[M, M, M](route, apiaction.Update, apiaction.UpdateBefore, apiaction.UpdateAfter)
	return func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
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
		if id == "" {
			return zero, c.missingID()
		}
		if err := a.updateFlow(c.ctx, c.serviceContext, id, m); err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}
