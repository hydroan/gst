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

func TestPresentQueryFields(t *testing.T) {
	t.Run("CollectsExplicitModelKeys", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"is_active": {"false"},
			"age":       {"0"},
			"isLocked":  {"true"},
			"size":      {"3"},
		})
		require.Equal(t, map[string]struct{}{
			"is_active": {},
			"age":       {},
			"is_locked": {},
			"size":      {},
		}, present, "camel case keys should normalize to snake case column names, and bare names like size are model filter columns")
	})

	t.Run("ExcludesFrameworkKeys", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"_page":         {"1"},
			"_size":         {"10"},
			"_limit":        {"100"},
			"_fuzzy":        {"true"},
			"_sort_by":      {"created_at desc"},
			"_no_total":     {"true"},
			"_cursor_value": {"abc"},
		})
		require.Empty(t, present, "framework parameters live in the underscore namespace and are not model filter columns")
	})

	t.Run("ExcludesKeysWithoutValues", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"is_active": {""},
			"remark":    {"", ""},
		})
		require.Empty(t, present, "an empty value means the caller is not filtering by that key")
	})
}
