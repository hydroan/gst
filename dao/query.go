package dao

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

func QueryModelsMap[M types.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M) (map[string]M, error) {
	return QueryModelsMapWithOptions[M](ctx, keyFunc, queryFunc, types.QueryOptions{AllowEmpty: true})
}

func QueryModelsMapWithOptions[M types.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M, opts types.QueryOptions) (map[string]M, error) {
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
