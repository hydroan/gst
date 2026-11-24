package controller

import (
	"context"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
)

// PatchManyCall returns the batch patch call of M on route, the counterpart
// of the handler PatchManyHandler returns for the generated handler of a
// PatchMany rpc: given the route parameters, the items the request message
// decoded into and the paths of each item's update mask, one mask per item
// in order (see maskFieldSet), it validates each item on the fields its mask
// names the way the handler validates the fields each item names (see
// validatePatchFields), refuses a versioned model whose item carries no
// version the way the handler does, runs the batch patch flow (see
// patchManyFlow) and answers with the records patched, or with the status
// the failure maps to (see call).
func PatchManyCall[M gst.Model](route string) func(ctx context.Context, params map[string]string, items []M, paths [][]string) ([]M, error) {
	a := newAction[M, M, M](route, apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
	return func(ctx context.Context, params map[string]string, items []M, paths [][]string) ([]M, error) {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return nil, c.invalid(err)
		}
		for i, item := range items {
			if reflect.ValueOf(item).IsNil() {
				return nil, c.invalid(errors.Newf("items[%d] carries no record", i))
			}
		}
		req := batch[M]{Items: items}
		normalizeBatch(&req)
		if len(paths) != len(req.Items) {
			return nil, c.invalid(errors.Newf("%d items carry %d update masks; each item names the fields to apply in a mask of its own", len(req.Items), len(paths)))
		}
		fieldSets := make([]patchFieldSet, len(paths))
		for i, itemPaths := range paths {
			fields, maskErr := maskFieldSet(a.typ, itemPaths)
			if maskErr != nil {
				return nil, c.invalid(errors.Wrapf(maskErr, "items[%d]", i))
			}
			fieldSets[i] = fields
		}
		for i, item := range req.Items {
			if versionField, omitted := versionOmitted(item, fieldSets[i]); omitted {
				return nil, c.refuse(databaseError(database.ErrVersionRequired), errors.Wrapf(database.ErrVersionRequired, "patch many %s item %d without its %s", a.name, i, versionField))
			}
		}
		for i, item := range req.Items {
			if err = validatePatchFields(item, fieldSets[i]); err != nil {
				return nil, c.invalidItemMessage(i, err)
			}
		}
		rsp, err := a.patchManyFlow(c.ctx, c.serviceContext, &req, fieldSets)
		if err != nil {
			return nil, c.fail(err)
		}
		return answer(c, rsp.Items)
	}
}
