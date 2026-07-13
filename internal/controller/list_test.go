package controller

import (
	"testing"

	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

type listQueryableTestModel struct {
	Name string `query:"name"`

	model.Query
	model.Base
}

type listUnsafeQueryableTestModel struct {
	Name string `query:"name"`

	model.Query
	model.UnsafeQuery
	model.Base
}

func TestDecodeListQueryGatesUnsafeQueryKeys(t *testing.T) {
	queryKeys := map[string][]string{
		"name":         {"alice"},
		"_fuzzy":       {"true"},
		"_sort_by":     {"created_at desc"},
		"_time_column": {"created_at"},
		"_start_time":  {"2025-01-01 00:00:00"},
		"_end_time":    {"2025-01-02 00:00:00"},
	}

	t.Run("QueryAcceptsRegularKeys", func(t *testing.T) {
		var m listQueryableTestModel
		require.NoError(t, decodeListQuery(&m, queryKeys))
	})

	t.Run("QueryRejectsUnsafeKeys", func(t *testing.T) {
		for _, key := range []string{"_or", "_index", "_select", "_no_cache", "_no_total"} {
			var m listQueryableTestModel
			err := decodeListQuery(&m, map[string][]string{key: {"true"}})
			require.Error(t, err, "unsafe query key %q must be rejected without model.UnsafeQuery", key)
		}
	})

	t.Run("UnsafeQueryAcceptsUnsafeKeys", func(t *testing.T) {
		var m listUnsafeQueryableTestModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{
			"_or":       {"true"},
			"_index":    {"idx_test"},
			"_select":   {"name"},
			"_no_cache": {"true"},
			"_no_total": {"true"},
		}))
	})

	t.Run("UnsafeQueryAloneRejectsRegularKeys", func(t *testing.T) {
		type unsafeOnlyModel struct {
			model.UnsafeQuery
			model.Base
		}
		var m unsafeOnlyModel
		require.Error(t, decodeListQuery(&m, map[string][]string{"_fuzzy": {"true"}}))
	})
}
