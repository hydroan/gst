package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
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
		"name":     {"alice"},
		"_sort_by": {"created_at desc"},
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
		require.Error(t, decodeListQuery(&m, map[string][]string{"_sort_by": {"created_at desc"}}))
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

	t.Run("ExcludesFieldConditionKeys", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"age[gt]": {"20"},
		})
		require.Empty(t, present, "field condition keys are not exact-filter columns")
	})
}

func TestParseQueryTime(t *testing.T) {
	t.Run("DateTimeLayout", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01 08:30:15", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 15, 0, time.Local), got)
	})

	t.Run("DateTimeLocalLayoutWithSeconds", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 15, 0, time.Local), got)
	})

	t.Run("DateTimeLocalLayoutWithoutSeconds", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 0, 0, time.Local), got)
	})

	t.Run("DateOnlyStartIsBeginOfDay", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), got)
	})

	t.Run("DateOnlyEndCoversWholeDay", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01", true)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond), got)
	})

	t.Run("RFC3339KeepsExplicitOffset", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15+08:00", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600))))
	})

	t.Run("RFC3339EndWithTimeOfDayIsNotExtended", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T00:00:00Z", true)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("UnixSeconds", func(t *testing.T) {
		got, err := parseQueryTime("1751328000", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Unix(1751328000, 0)))
	})

	t.Run("UnixMilliseconds", func(t *testing.T) {
		got, err := parseQueryTime("1751328000123", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.UnixMilli(1751328000123)))
	})

	t.Run("UnsupportedFormatFails", func(t *testing.T) {
		_, err := parseQueryTime("07/01/2026", false)
		require.Error(t, err)
	})
}

type expandQueryTestModel struct {
	Children   []*expandQueryTestModel
	Parent     *expandQueryTestModel
	ChildItems []*expandQueryTestModel

	model.Base
}

func (*expandQueryTestModel) Expands() []string { return []string{"Children", "Parent", "ChildItems"} }

func TestParseExpandQuery(t *testing.T) {
	t.Run("DepthRepeatsSliceExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=3")
		require.Equal(t, []string{"Children.Children.Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NonSliceExpandIgnoresDepth", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Parent&_depth=3")
		require.Equal(t, []string{"Parent"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("AllSelectsEveryModelExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=all")
		require.Equal(t, []string{"Children", "Parent", "ChildItems"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("ExpandMatchesCaseInsensitively", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=children")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("ExpandMatchesSnakeCaseName", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=child_items")
		require.Equal(t, []string{"ChildItems"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("DepthAcceptsUpperBoundTen", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=10")
		require.Equal(t, []string{strings.Repeat("Children.", 9) + "Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("UnknownExpandDropped", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Bogus")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("OutOfRangeDepthFallsBackToOne", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=11")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NoExpandParameterReturnsNothing", func(t *testing.T) {
		c := newTestGetContext(t, "/items")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})
}

type conditionQueryTestModel struct {
	Name      string    `query:"name"`
	Age       int       `json:"age"`
	Remark    string    `json:"remark"`
	ItemCount int       `json:"item_count"`
	Enabled   bool      `json:"enabled"`
	ExpiredAt time.Time `json:"expired_at"`

	model.Query
	model.Base
}

func TestParseFieldConditionsQuery(t *testing.T) {
	t.Run("ExtractsOperatorConditionsAndIgnoresOtherKeys", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[gt]":      {"20"},
			"remark[like]": {"hello"},
			"name":         {"alice"},
			"_sort_by":     {"created_at desc"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{
			{Column: "age", Op: types.FilterOpGt, Value: "20"},
			{Column: "remark", Op: types.FilterOpLike, Value: "hello"},
		}, conds)
	})

	t.Run("CoexistsWithExactFilterOnSameField", func(t *testing.T) {
		query := map[string][]string{
			"age":     {"10"},
			"age[gt]": {"20"},
		}
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, query)
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "age", Op: types.FilterOpGt, Value: "20"}}, conds)

		var m conditionQueryTestModel
		require.NoError(t, decodeListQuery(&m, query))
		require.Equal(t, 10, m.Age, "bare key keeps feeding the exact business filter")
	})

	t.Run("MapsCamelFieldToSnakeColumn", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"itemCount[notlike]": {"sample"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "item_count", Op: types.FilterOpNotLike, Value: "sample"}}, conds)
	})

	t.Run("AcceptsBaseLiftedColumns", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"id[notin]": {"a,b"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "id", Op: types.FilterOpNotIn, Value: "a,b"}}, conds)
	})

	t.Run("SkipsEmptyValues", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[gt]": {""},
		})
		require.NoError(t, err)
		require.Empty(t, conds)
	})

	t.Run("RejectsUnknownField", func(t *testing.T) {
		_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"bogus[gt]": {"1"},
		})
		require.Error(t, err)
	})

	t.Run("RejectsUnknownOperator", func(t *testing.T) {
		_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[regex]": {"1"},
		})
		require.Error(t, err)
	})

	t.Run("RejectsMalformedKeys", func(t *testing.T) {
		for _, key := range []string{"age[gt", "age[]", "age[gt]x", "age[gt][lt]", "[gt]"} {
			_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{key: {"1"}})
			require.Error(t, err, "key %q must be rejected", key)
		}
	})

	t.Run("RequiresModelQuery", func(t *testing.T) {
		type plainModel struct {
			Age int `json:"age"`

			model.Base
		}
		_, err := parseFieldConditionsQuery(&plainModel{}, map[string][]string{
			"age[gt]": {"1"},
		})
		require.Error(t, err)
	})

	t.Run("TimeFieldNormalizesFlexibleFormats", func(t *testing.T) {
		for key, want := range map[string]string{
			// A date-only lower bound starts at the beginning of the day.
			"expired_at[gte]": time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(fieldConditionTimeLayout),
			// A date-only inclusive upper bound covers the whole day.
			"expired_at[lte]": time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).Format(fieldConditionTimeLayout),
			// A date-only exclusive lower bound means "after the whole day".
			"expired_at[gt]": time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).Format(fieldConditionTimeLayout),
			// A date-only exclusive upper bound means "before the day starts".
			"expired_at[lt]": time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(fieldConditionTimeLayout),
		} {
			conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{key: {"2026-07-01"}})
			require.NoError(t, err, "key %q", key)
			require.Len(t, conds, 1)
			require.Equal(t, "expired_at", conds[0].Column)
			require.Equal(t, want, conds[0].Value, "key %q", key)
		}

		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"expired_at[eq]": {"2026-07-01T08:30:15+08:00"},
		})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t,
			time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).In(time.Local).Format(fieldConditionTimeLayout),
			conds[0].Value, "an explicit offset must be converted to the server's local zone")
	})

	t.Run("TimeFieldRejectsInvalidValue", func(t *testing.T) {
		_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"expired_at[gte]": {"07/01/2026"},
		})
		require.Error(t, err)
	})

	t.Run("TimeFieldRejectsSetAndSubstringOps", func(t *testing.T) {
		for _, key := range []string{"expired_at[like]", "expired_at[notlike]", "expired_at[in]", "expired_at[notin]", "expired_at[startswith]", "expired_at[endswith]"} {
			_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{key: {"2026-07-01"}})
			require.Error(t, err, "key %q must be rejected on a time field", key)
		}
	})

	t.Run("PrefixAndSuffixOpsPassStringValues", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"remark[endswith]":   {"suffix"},
			"remark[startswith]": {"prefix"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{
			{Column: "remark", Op: types.FilterOpEndsWith, Value: "suffix"},
			{Column: "remark", Op: types.FilterOpStartsWith, Value: "prefix"},
		}, conds)

		_, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"enabled[startswith]": {"tr"},
		})
		require.Error(t, err, "prefix matching makes no sense on a bool field")
	})

	t.Run("IsNullWorksOnAnyColumnWithBoolValue", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"expired_at[isnull]": {"false"},
			"remark[isnull]":     {"true"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{
			{Column: "expired_at", Op: types.FilterOpIsNull, Value: "0"},
			{Column: "remark", Op: types.FilterOpIsNull, Value: "1"},
		}, conds)

		_, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"remark[isnull]": {"yes"},
		})
		require.Error(t, err, "isnull requires a boolean value")
	})

	t.Run("BaseTimeColumnsFilterable", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"created_at[gte]": {"2026-07-01"},
			"updated_at[lt]":  {"2026-07-15"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{
			{Column: "created_at", Op: types.FilterOpGte, Value: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(fieldConditionTimeLayout)},
			{Column: "updated_at", Op: types.FilterOpLt, Value: time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local).Format(fieldConditionTimeLayout)},
		}, conds)
	})

	t.Run("NumericFieldValidatesValues", func(t *testing.T) {
		_, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[gt]": {"abc"},
		})
		require.Error(t, err, "non-numeric comparison value must be rejected")

		_, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[in]": {"1,x"},
		})
		require.Error(t, err, "every set member must be numeric")

		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"age[in]": {"1,2"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "age", Op: types.FilterOpIn, Value: "1,2"}}, conds)
	})

	t.Run("BoolFieldNormalizesAndGatesOps", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"enabled[eq]": {"true"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "enabled", Op: types.FilterOpEq, Value: "1"}}, conds)

		conds, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"enabled[ne]": {"0"},
		})
		require.NoError(t, err)
		require.Equal(t, []types.FieldCondition{{Column: "enabled", Op: types.FilterOpNe, Value: "0"}}, conds)

		_, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"enabled[gt]": {"true"},
		})
		require.Error(t, err, "ordering operators make no sense on a bool field")

		_, err = parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"enabled[eq]": {"yes"},
		})
		require.Error(t, err, "non-boolean value must be rejected")
	})

	t.Run("RejectsCombinationWithOr", func(t *testing.T) {
		type unsafeConditionModel struct {
			Age int `json:"age"`

			model.Query
			model.UnsafeQuery
			model.Base
		}
		_, err := parseFieldConditionsQuery(&unsafeConditionModel{}, map[string][]string{
			"age[gt]": {"1"},
			"_or":     {"true"},
		})
		require.Error(t, err, "flat OR building cannot express (a OR b) AND cond, so the combination must fail closed")

		conds, err := parseFieldConditionsQuery(&unsafeConditionModel{}, map[string][]string{
			"age[gt]": {"1"},
			"_or":     {"false"},
		})
		require.NoError(t, err)
		require.Len(t, conds, 1)
	})

	t.Run("LeavesFrameworkNamespaceAlone", func(t *testing.T) {
		conds, err := parseFieldConditionsQuery(&conditionQueryTestModel{}, map[string][]string{
			"_page[gt]": {"1"},
		})
		require.NoError(t, err)
		require.Empty(t, conds, "underscore keys stay in the framework namespace and are not field conditions")
	})
}

func TestDecodeListQueryPageSizeGating(t *testing.T) {
	type cursorOnlyModel struct {
		Name string `query:"name"`

		model.Cursor
		model.Base
	}
	type paginatableModel struct {
		Name string `query:"name"`

		model.Pagination
		model.Base
	}
	type plainModel struct {
		Name string `query:"name"`

		model.Base
	}

	t.Run("CursorModelAcceptsSizeButRejectsPage", func(t *testing.T) {
		var m cursorOnlyModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{"_size": {"50"}}),
			"cursor pagination needs a client-adjustable batch size")
		require.Error(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}}),
			"offset paging conflicts with cursor semantics")
	})

	t.Run("PaginatableModelAcceptsBoth", func(t *testing.T) {
		var m paginatableModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}, "_size": {"50"}}))
	})

	t.Run("PlainModelRejectsBoth", func(t *testing.T) {
		var m plainModel
		require.Error(t, decodeListQuery(&m, map[string][]string{"_size": {"50"}}))
		require.Error(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}}))
	})
}

func TestResolveListPagination(t *testing.T) {
	t.Run("AdjustableDefaultsAndClamp", func(t *testing.T) {
		page, size := resolveListPagination(0, 0, true, false)
		require.Equal(t, 0, page)
		require.Equal(t, defaultPageSize, size, "adjustable models default to a small page")

		_, size = resolveListPagination(0, 50, true, false)
		require.Equal(t, 50, size)

		_, size = resolveListPagination(0, maxPageSize+1, true, false)
		require.Equal(t, maxPageSize, size, "oversized page size clamps to the cap")
	})

	t.Run("NonAdjustableKeepsBottomLine", func(t *testing.T) {
		_, size := resolveListPagination(0, 0, false, false)
		require.Equal(t, defaultLimit, size, "models without client size control keep the full-table safety limit")
	})

	t.Run("ActiveCursorIgnoresPage", func(t *testing.T) {
		page, _ := resolveListPagination(3, 50, true, true)
		require.Equal(t, 1, page, "offset paging must not stack on top of an active cursor")
	})
}

func TestDecodeListQueryIgnoresFieldConditionKeys(t *testing.T) {
	var m conditionQueryTestModel
	require.NoError(t, decodeListQuery(&m, map[string][]string{
		"name":    {"alice"},
		"age[gt]": {"20"},
	}))
	require.Equal(t, "alice", m.Name)
}

// newTestGetContext builds a gin context carrying a GET request with the given
// target URL, for exercising query-parameter parsing helpers.
func newTestGetContext(t *testing.T, target string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}
