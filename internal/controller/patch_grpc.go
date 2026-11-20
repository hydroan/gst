package controller

import (
	"context"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/types"
)

// PatchCall returns the patch call of M on route, the counterpart of the
// handler PatchHandler returns for the generated handler of a Patch rpc:
// given the route parameters, the id the request message names the record
// by, the values it decoded into and the paths of its update mask, which
// name the fields to apply as the message names them (see maskFieldSet), it
// refuses a message carrying no record the way the handler refuses a
// request without a body, validates the fields the mask names the way the
// handler validates the fields a body names (see validatePatchFields),
// refuses a versioned model patched without its version the way the
// handler does, runs the patch flow (see patchFlow) and answers with the
// record patched, or with the status the failure maps to (see call).
func PatchCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M, paths []string) (M, error) {
	a := newAction[M, M, M](route, apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter)
	return func(ctx context.Context, params map[string]string, id string, m M, paths []string) (M, error) {
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
		fields, err := maskFieldSet(a.typ, paths)
		if err != nil {
			return zero, c.invalid(err)
		}
		if versionField, omitted := versionOmitted(m, fields); omitted {
			return zero, c.refuse(databaseError(database.ErrVersionRequired), errors.Wrapf(database.ErrVersionRequired, "patch %s without its %s", a.name, versionField))
		}
		if err = validatePatchFields(m, fields); err != nil {
			return zero, c.invalidMessage(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		cur, err := a.patchFlow(c.ctx, c.serviceContext, id, m, fields)
		if err != nil {
			return zero, c.fail(err)
		}
		return answer(c, cur)
	}
}
