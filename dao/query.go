package dao

import (
	"context"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

func QueryModelsMap[M types.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M) (map[string]M, error) {
	return QueryModelsMapWithConfig[M](ctx, keyFunc, queryFunc, types.QueryConfig{AllowEmpty: true})
}

func QueryModelsMapWithConfig[M types.Model](ctx context.Context, keyFunc func(M) string, queryFunc func() M, config types.QueryConfig) (map[string]M, error) {
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
		WithQuery(queryFunc(), config).
		List(&objs); err != nil {
		return nil, err
	}

	objMap := make(map[string]M)
	for _, obj := range objs {
		objMap[keyFunc(obj)] = obj
	}

	return objMap, nil
}
