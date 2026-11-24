// Package dao holds helpers that load records into lookup structures.
package dao

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/gst"
)

// QueryModelsMap lists the records of M matching the model queryFunc returns
// and returns them keyed by keyFunc. A nil keyFunc keys by the record id; a
// nil queryFunc filters by the zero model. The query runs with AllowEmpty, so
// a model naming no condition lists every record of the table. Records
// sharing a key keep the one listed last.
func QueryModelsMap[M gst.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M) (map[string]M, error) {
	return QueryModelsMapWithOptions[M](ctx, keyFunc, queryFunc, gst.QueryOptions{AllowEmpty: true})
}

// QueryModelsMapWithOptions is QueryModelsMap under the query options the
// caller gives in place of AllowEmpty: under the zero options a model naming
// no condition matches no record.
func QueryModelsMapWithOptions[M gst.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M, opts gst.QueryOptions) (map[string]M, error) {
	if keyFunc == nil {
		keyFunc = func(m M) string {
			return m.GetID()
		}
	}
	if queryFunc == nil {
		queryFunc = func() M {
			var m M
			return m
		}
	}

	objs := make([]M, 0)
	if err := database.Database[M](ctx).
		WithQuery(queryFunc(), opts).
		List(&objs); err != nil {
		return nil, err
	}

	objMap := make(map[string]M)
	for _, obj := range objs {
		objMap[keyFunc(obj)] = obj
	}

	return objMap, nil
}
