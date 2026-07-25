package urlquery

import (
	"net/url"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type filterTestModel struct {
	Name      string    `query:"name"`
	Age       int       `json:"age"`
	Remark    string    `json:"remark"`
	ItemCount int       `json:"item_count"`
	Enabled   bool      `json:"enabled"`
	ExpiredAt time.Time `json:"expired_at"`
	// GroupIDs and Renamed anchor the column resolution: gorm renders the
	// former as group_ids (a plain snake case conversion would not) and the
	// latter through its column tag, while the URL keeps the json name.
	GroupIDs string `json:"group_ids"`
	Renamed  string `json:"renamed" gorm:"column:custom_column"`

	modelregistry.Query
	modelregistry.Base
}

func TestFilters(t *testing.T) {
	t.Run("ExtractsOperatorConditionsAndIgnoresOtherKeys", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"age[gt]":      {"20"},
			"remark[like]": {"hello"},
			"name":         {"alice"},
			"_sort_by":     {"created_at desc"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "age", Op: types.FilterOpGt, Value: "20"},
			{Column: "remark", Op: types.FilterOpLike, Value: "hello"},
		}, conds)
	})

	t.Run("CoexistsWithExactFilterOnSameField", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"age":     {"10"},
			"age[gt]": {"20"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "age", Op: types.FilterOpGt, Value: "20"}}, conds,
			"the bare key stays with the exact business filter and only the operator key becomes a filter")
	})

	t.Run("MapsCamelFieldToSnakeColumn", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"itemCount[notlike]": {"sample"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "item_count", Op: types.FilterOpNotLike, Value: "sample"}}, conds)
	})

	t.Run("AcceptsBaseLiftedColumns", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"id[notin]": {"a,b"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "id", Op: types.FilterOpNotIn, Value: []string{"a", "b"}}}, conds)
	})

	t.Run("UsesGormColumnNames", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"group_ids[in]": {"a,b"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "group_ids", Op: types.FilterOpIn, Value: []string{"a", "b"}},
		}, conds, "gorm renders GroupIDs as group_ids, not group_i_ds")

		conds, err = Filters(url.Values{
			"renamed[eq]": {"x"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "custom_column", Op: types.FilterOpEq, Value: "x"},
		}, conds, "the URL keeps the json name while SQL uses the column tag")
	})

	t.Run("SkipsEmptyValues", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"age[gt]": {""},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Empty(t, conds)
	})

	t.Run("RejectsUnknownField", func(t *testing.T) {
		_, err := Filters(url.Values{
			"bogus[gt]": {"1"},
		}, &filterTestModel{})
		require.Error(t, err)
	})

	t.Run("RejectsUnknownOperator", func(t *testing.T) {
		_, err := Filters(url.Values{
			"age[regex]": {"1"},
		}, &filterTestModel{})
		require.Error(t, err)
	})

	t.Run("RejectsMalformedKeys", func(t *testing.T) {
		for _, key := range []string{"age[gt", "age[]", "age[gt]x", "age[gt][lt]", "[gt]"} {
			_, err := Filters(url.Values{key: {"1"}}, &filterTestModel{})
			require.Error(t, err, "key %q must be rejected", key)
		}
	})

	t.Run("RequiresModelQuery", func(t *testing.T) {
		type plainModel struct {
			Age int `json:"age"`

			modelregistry.Base
		}
		_, err := Filters(url.Values{
			"age[gt]": {"1"},
		}, &plainModel{})
		require.Error(t, err)
	})

	t.Run("TimeFieldNormalizesFlexibleFormats", func(t *testing.T) {
		for key, want := range map[string]string{
			// A date-only lower bound starts at the beginning of the day.
			"expired_at[gte]": time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(filterTimeLayout),
			// A date-only inclusive upper bound covers the whole day.
			"expired_at[lte]": time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).Format(filterTimeLayout),
			// A date-only exclusive lower bound means "after the whole day".
			"expired_at[gt]": time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).Format(filterTimeLayout),
			// A date-only exclusive upper bound means "before the day starts".
			"expired_at[lt]": time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(filterTimeLayout),
		} {
			conds, err := Filters(url.Values{key: {"2026-07-01"}}, &filterTestModel{})
			require.NoError(t, err, "key %q", key)
			require.Len(t, conds, 1)
			require.Equal(t, "expired_at", conds[0].Column)
			require.Equal(t, want, conds[0].Value, "key %q", key)
		}

		conds, err := Filters(url.Values{
			"expired_at[eq]": {"2026-07-01T08:30:15+08:00"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t,
			time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).In(time.Local).Format(filterTimeLayout),
			conds[0].Value, "an explicit offset must be converted to the server's local zone")
	})

	t.Run("TimeFieldRejectsInvalidValue", func(t *testing.T) {
		_, err := Filters(url.Values{
			"expired_at[gte]": {"07/01/2026"},
		}, &filterTestModel{})
		require.Error(t, err)
	})

	t.Run("TimeFieldRejectsSetAndSubstringOps", func(t *testing.T) {
		for _, key := range []string{"expired_at[like]", "expired_at[notlike]", "expired_at[in]", "expired_at[notin]", "expired_at[startswith]", "expired_at[endswith]"} {
			_, err := Filters(url.Values{key: {"2026-07-01"}}, &filterTestModel{})
			require.Error(t, err, "key %q must be rejected on a time field", key)
		}
	})

	t.Run("PrefixAndSuffixOpsPassStringValues", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"remark[endswith]":   {"suffix"},
			"remark[startswith]": {"prefix"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "remark", Op: types.FilterOpEndsWith, Value: "suffix"},
			{Column: "remark", Op: types.FilterOpStartsWith, Value: "prefix"},
		}, conds)

		_, err = Filters(url.Values{
			"enabled[startswith]": {"tr"},
		}, &filterTestModel{})
		require.Error(t, err, "prefix matching makes no sense on a bool field")
	})

	t.Run("IsNullWorksOnAnyColumnWithBoolValue", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"expired_at[isnull]": {"false"},
			"remark[isnull]":     {"true"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "expired_at", Op: types.FilterOpIsNull, Value: false},
			{Column: "remark", Op: types.FilterOpIsNull, Value: true},
		}, conds)

		_, err = Filters(url.Values{
			"remark[isnull]": {"yes"},
		}, &filterTestModel{})
		require.Error(t, err, "isnull requires a boolean value")
	})

	t.Run("BareBaseTimestampKeyBecomesEqCondition", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"created_at": {"2026-07-01 08:00:00"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "created_at", Op: types.FilterOpEq, Value: time.Date(2026, 7, 1, 8, 0, 0, 0, time.Local).Format(filterTimeLayout)},
		}, conds, "the bare framework timestamp key is an exact-match filter, consistent with every other documented parameter")

		conds, err = Filters(url.Values{
			"updated_at": {""},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Empty(t, conds, "an empty value means not filtering")

		_, err = Filters(url.Values{
			"created_at": {"not-a-time"},
		}, &filterTestModel{})
		require.Error(t, err, "the exact-match value still goes through time validation")
	})

	t.Run("BaseTimeColumnsFilterable", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"created_at[gte]": {"2026-07-01"},
			"updated_at[lt]":  {"2026-07-15"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "created_at", Op: types.FilterOpGte, Value: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local).Format(filterTimeLayout)},
			{Column: "updated_at", Op: types.FilterOpLt, Value: time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local).Format(filterTimeLayout)},
		}, conds)
	})

	t.Run("NumericFieldValidatesValues", func(t *testing.T) {
		_, err := Filters(url.Values{
			"age[gt]": {"abc"},
		}, &filterTestModel{})
		require.Error(t, err, "non-numeric comparison value must be rejected")

		_, err = Filters(url.Values{
			"age[in]": {"1,x"},
		}, &filterTestModel{})
		require.Error(t, err, "every set member must be numeric")

		conds, err := Filters(url.Values{
			"age[in]": {"1,2"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "age", Op: types.FilterOpIn, Value: []string{"1", "2"}}}, conds)
	})

	t.Run("BoolFieldNormalizesAndGatesOps", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"enabled[eq]": {"true"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "enabled", Op: types.FilterOpEq, Value: true}}, conds)

		conds, err = Filters(url.Values{
			"enabled[ne]": {"0"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "enabled", Op: types.FilterOpNe, Value: false}}, conds)

		_, err = Filters(url.Values{
			"enabled[gt]": {"true"},
		}, &filterTestModel{})
		require.Error(t, err, "ordering operators make no sense on a bool field")

		_, err = Filters(url.Values{
			"enabled[eq]": {"yes"},
		}, &filterTestModel{})
		require.Error(t, err, "non-boolean value must be rejected")
	})

	t.Run("LeavesFrameworkNamespaceAlone", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"_page[gt]": {"1"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Empty(t, conds, "underscore keys stay in the framework namespace and are not filters")
	})
}

func TestWithoutFilters(t *testing.T) {
	q := url.Values{
		"name":            {"alice"},
		"age[gt]":         {"20"},
		"created_at":      {"2026-07-01"},
		"updated_at[lte]": {"2026-07-15"},
		"_page":           {"2"},
		"_page[gt]":       {"1"},
	}
	require.Equal(t, url.Values{
		"name":      {"alice"},
		"_page":     {"2"},
		"_page[gt]": {"1"},
	}, withoutFilters(q), "operator keys and bare framework timestamps belong to Filters, underscore keys never do")

	require.Len(t, q, 6, "the input query must be left untouched")
}

func TestDecode(t *testing.T) {
	t.Run("FillsModelFieldsByQueryTag", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"name": {"alice"}, "age": {"10"}}, &m))
		require.Equal(t, "alice", m.Name)
		require.Equal(t, 10, m.Age, "fields are mapped by the query alias tag, falling back to the field name")
	})

	t.Run("KeepsFilterKeysAwayFromTheDecoder", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{
			"name":            {"alice"},
			"age":             {"10"},
			"age[gt]":         {"20"},
			"created_at":      {"2026-07-01"},
			"updated_at[lte]": {"2026-07-15"},
		}, &m))
		require.Equal(t, "alice", m.Name)
		require.Equal(t, 10, m.Age, "the bare key keeps feeding the exact filter while the operator key is left to Filters")
	})

	t.Run("FillsFrameworkQueryFields", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"_page": {"2"}, "_size": {"50"}, "_sort_by": {"created_at desc"}}, &m))
		require.Equal(t, 2, m.Page)
		require.Equal(t, 50, m.Size)
		require.Equal(t, "created_at desc", m.SortBy)
	})

	t.Run("DropsSizeWhenTheModelCannotCarryIt", func(t *testing.T) {
		var m cursorTestModel
		require.NoError(t, Decode(url.Values{"_size": {"50"}}, &m),
			"a cursor-only model accepts _size as its batch size but has no Size field to decode it into")
	})

	t.Run("RejectsUnknownKeys", func(t *testing.T) {
		var m filterTestModel
		require.Error(t, Decode(url.Values{"bogus": {"1"}}, &m),
			"an unknown key is a typo and must not silently widen the result set")
		require.Error(t, Decode(url.Values{"_bogus": {"1"}}, &m),
			"a mistyped framework parameter must be reported too")
	})
}

func TestPresentFields(t *testing.T) {
	t.Run("CollectsExplicitModelKeys", func(t *testing.T) {
		present := PresentFields(url.Values{
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
		present := PresentFields(url.Values{
			"_page":         {"1"},
			"_size":         {"10"},
			"_limit":        {"100"},
			"_sort_by":      {"created_at desc"},
			"_expand":       {"all"},
			"_cursor_value": {"abc"},
		})
		require.Empty(t, present, "framework parameters live in the underscore namespace and are not model filter columns")
	})

	t.Run("ExcludesKeysWithoutValues", func(t *testing.T) {
		present := PresentFields(url.Values{
			"is_active": {""},
			"remark":    {"", ""},
		})
		require.Empty(t, present, "an empty value means the caller is not filtering by that key")
	})

	t.Run("ExcludesFilterKeys", func(t *testing.T) {
		present := PresentFields(url.Values{
			"age[gt]": {"20"},
		})
		require.Empty(t, present, "filter keys are not exact-filter columns")
	})
}

type paginatableTestModel struct {
	modelregistry.Pagination
	modelregistry.Base
}

type cursorTestModel struct {
	modelregistry.Cursor
	modelregistry.Base
}

type plainTestModel struct {
	modelregistry.Base
}

func TestPagination(t *testing.T) {
	t.Run("PaginatableModelReadsBothParameters", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {"2"}, "_size": {"50"}}, &paginatableTestModel{})
		require.Equal(t, 2, page)
		require.Equal(t, 50, size)
	})

	t.Run("PaginatableModelDefaultsAndClamps", func(t *testing.T) {
		page, size := Pagination(url.Values{}, &paginatableTestModel{})
		require.Equal(t, 0, page)
		require.Equal(t, defaultPageSize, size, "adjustable models default to a small page")

		_, size = Pagination(url.Values{"_size": {"0"}}, &paginatableTestModel{})
		require.Equal(t, defaultPageSize, size)

		_, size = Pagination(url.Values{"_size": {"101"}}, &paginatableTestModel{})
		require.Equal(t, maxPageSize, size, "oversized page size clamps to the cap")
	})

	t.Run("UnparsableValuesFallBackToDefaults", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {""}, "_size": {"abc"}}, &paginatableTestModel{})
		require.Equal(t, 0, page)
		require.Equal(t, defaultPageSize, size)
	})

	t.Run("CursorModelIgnoresPageButKeepsSize", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {"2"}, "_size": {"50"}}, &cursorTestModel{})
		require.Equal(t, 0, page, "offset paging conflicts with cursor semantics and is not offered to cursor models")
		require.Equal(t, 50, size, "cursor pagination needs a client-adjustable batch size")
	})

	t.Run("PlainModelKeepsBottomLine", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {"2"}, "_size": {"50"}}, &plainTestModel{})
		require.Equal(t, 0, page)
		require.Equal(t, defaultLimit, size, "models without client size control keep the full-table safety limit")
	})

	t.Run("ActiveCursorResetsPage", func(t *testing.T) {
		page, _ := Pagination(url.Values{"_page": {"3"}, "_cursor_value": {"abc"}}, &filterTestModel{})
		require.Equal(t, 1, page, "offset paging must not stack on top of an active cursor")

		page, _ = Pagination(url.Values{"_page": {"3"}, "_cursor_value": {"abc"}}, &plainTestModel{})
		require.Equal(t, 0, page, "a model without cursor support never sees a cursor")
	})
}

func TestCursor(t *testing.T) {
	t.Run("CursorModelReadsAllThreeParameters", func(t *testing.T) {
		value, next, field := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"created_at"},
		}, &cursorTestModel{})
		require.Equal(t, "abc", value)
		require.True(t, next)
		require.Equal(t, "created_at", field)
	})

	t.Run("MissingDirectionMeansPrevious", func(t *testing.T) {
		_, next, _ := Cursor(url.Values{"_cursor_value": {"abc"}}, &cursorTestModel{})
		require.False(t, next)
	})

	t.Run("ModelWithoutCursorYieldsZeroCursor", func(t *testing.T) {
		value, next, field := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"created_at"},
		}, &plainTestModel{})
		require.Empty(t, value, "a zero cursor makes WithCursor a no-op")
		require.False(t, next)
		require.Empty(t, field)
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
