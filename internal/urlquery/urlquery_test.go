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

	// Filter groups are a service-side capability: letting a client build them
	// would hand back the ability to combine conditions with OR, which is what
	// the removed _or parameter got wrong.
	t.Run("RejectsGroupOperators", func(t *testing.T) {
		for _, key := range []string{"age[or]", "age[and]"} {
			_, err := Filters(url.Values{key: {"1"}}, &filterTestModel{})
			require.Error(t, err, "group operator %q must not be reachable from a URL", key)
		}
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

	t.Run("TimeFieldAcceptsOnlyRFC3339", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"expired_at[gte]": {"2026-07-01T00:00:00Z"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t, "expired_at", conds[0].Column)
		require.Equal(t,
			time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout),
			conds[0].Value, "the bound travels as the UTC wall clock")

		conds, err = Filters(url.Values{
			"expired_at[eq]": {"2026-07-01T08:30:15+08:00"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t,
			time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).UTC().Format(types.FilterTimeLayout),
			conds[0].Value, "an explicit offset must be converted to UTC")

		// A value without an explicit offset names a different instant per
		// server zone, so it is rejected instead of being guessed at.
		for _, value := range []string{"2026-07-01", "2026-07-01 08:30:15", "2026-07-01T08:30:15"} {
			_, err := Filters(url.Values{"expired_at[gte]": {value}}, &filterTestModel{})
			require.Error(t, err, "zone-less value %q must be rejected", value)
		}
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
			"created_at": {"2026-07-01T08:00:00Z"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "created_at", Op: types.FilterOpEq, Value: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)},
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
			"created_at[gte]": {"2026-07-01T00:00:00Z"},
			"updated_at[lt]":  {"2026-07-15T00:00:00Z"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			{Column: "created_at", Op: types.FilterOpGte, Value: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)},
			{Column: "updated_at", Op: types.FilterOpLt, Value: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)},
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

func TestOrders(t *testing.T) {
	t.Run("SingleColumnDefaultsToAscending", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"name"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{{Column: "name", Direction: types.OrderAsc}}, orders)
	})

	t.Run("DirectionIsCaseInsensitive", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"name DESC"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{{Column: "name", Direction: types.OrderDesc}}, orders)
	})

	t.Run("MultipleTermsKeepTheirOrder", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"age desc, name asc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{
			{Column: "age", Direction: types.OrderDesc},
			{Column: "name", Direction: types.OrderAsc},
		}, orders)
	})

	t.Run("ColumnCarriesTheDatabaseName", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"renamed desc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{{Column: "custom_column", Direction: types.OrderDesc}}, orders,
			"the URL names a column by its query name, but ORDER BY needs the database name")
	})

	t.Run("BaseTimestampIsSortable", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"created_at desc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{{Column: "created_at", Direction: types.OrderDesc}}, orders,
			`query:"-" only opts the timestamp out of exact filtering, the json name still resolves it`)
	})

	t.Run("NonFilterableColumnFails", func(t *testing.T) {
		_, err := Orders(url.Values{"_sort_by": {"deleted_at"}}, &filterTestModel{})
		require.Error(t, err, "a column hidden from JSON is framework bookkeeping and is not sortable either")
	})

	t.Run("UnknownColumnFails", func(t *testing.T) {
		_, err := Orders(url.Values{"_sort_by": {"no_such_column"}}, &filterTestModel{})
		require.Error(t, err, "an unknown sort column must fail instead of reaching the database")
	})

	t.Run("UnknownDirectionFails", func(t *testing.T) {
		_, err := Orders(url.Values{"_sort_by": {"name sideways"}}, &filterTestModel{})
		require.Error(t, err)
	})

	t.Run("MalformedTermFails", func(t *testing.T) {
		_, err := Orders(url.Values{"_sort_by": {"name asc extra"}}, &filterTestModel{})
		require.Error(t, err)
	})

	t.Run("MissingParameterYieldsNoOrder", func(t *testing.T) {
		orders, err := Orders(url.Values{}, &filterTestModel{})
		require.NoError(t, err)
		require.Empty(t, orders)
	})

	t.Run("ModelWithoutQueryYieldsNoOrder", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"name"}}, &plainTestModel{})
		require.NoError(t, err)
		require.Empty(t, orders, "a model that did not opt in to model.Query is not sortable through the URL")
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

type cursorAutoTestModel struct {
	modelregistry.Cursor
	modelregistry.AutoBase
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
		require.Equal(t, 1, page, "an unset page normalizes to the first page")
		require.Equal(t, defaultPageSize, size, "adjustable models default to a small page")

		page, _ = Pagination(url.Values{"_page": {"-5"}}, &paginatableTestModel{})
		require.Equal(t, 1, page, "a non-positive page normalizes to the first page")

		_, size = Pagination(url.Values{"_size": {"0"}}, &paginatableTestModel{})
		require.Equal(t, defaultPageSize, size)

		_, size = Pagination(url.Values{"_size": {"101"}}, &paginatableTestModel{})
		require.Equal(t, maxPageSize, size, "oversized page size clamps to the cap")
	})

	t.Run("UnparsableValuesFallBackToDefaults", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {""}, "_size": {"abc"}}, &paginatableTestModel{})
		require.Equal(t, 1, page, "an unparsable page normalizes to the first page")
		require.Equal(t, defaultPageSize, size)
	})

	t.Run("CursorModelIgnoresPageButKeepsSize", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {"2"}, "_size": {"50"}}, &cursorTestModel{})
		require.Equal(t, 1, page, "offset paging conflicts with cursor semantics and is not offered to cursor models; the page still normalizes to 1")
		require.Equal(t, 50, size, "cursor pagination needs a client-adjustable batch size")
	})

	t.Run("PlainModelKeepsBottomLine", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {"2"}, "_size": {"50"}}, &plainTestModel{})
		require.Equal(t, 1, page, "a model without offset paging still yields a usable first page")
		require.Equal(t, defaultLimit, size, "models without client size control keep the full-table safety limit")
	})

	t.Run("ActiveCursorResetsPage", func(t *testing.T) {
		page, _ := Pagination(url.Values{"_page": {"3"}, "_cursor_value": {"abc"}}, &filterTestModel{})
		require.Equal(t, 1, page, "offset paging must not stack on top of an active cursor")

		page, _ = Pagination(url.Values{"_page": {"3"}, "_cursor_value": {"abc"}}, &plainTestModel{})
		require.Equal(t, 1, page, "a model without cursor support never sees a cursor; the page still normalizes to 1")
	})
}

func TestCursor(t *testing.T) {
	t.Run("CursorModelReadsAllThreeParameters", func(t *testing.T) {
		cursor, err := Cursor(url.Values{
			"_cursor_value": {"2026-07-01T08:30:15+08:00"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"created_at"},
		}, &cursorTestModel{})
		require.NoError(t, err)
		// A time boundary is normalized like a time filter bound: the RFC 3339
		// input travels as the UTC wall clock.
		boundary := time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).UTC().Format(types.FilterTimeLayout)
		require.Equal(t, types.CursorForward(types.Asc("created_at"), boundary), cursor)
	})

	t.Run("MissingDirectionTravelsBackward", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_value": {"abc"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Backward)
		require.Empty(t, cursor.Order.Column, "an unnamed column leaves the primary key fallback to the database layer")
	})

	t.Run("UnknownColumnFails", func(t *testing.T) {
		_, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_field": {"no_such_column"},
		}, &cursorTestModel{})
		require.Error(t, err, "an unknown cursor column must fail instead of reaching the database")
	})

	t.Run("MistypedValueOnNumericDefaultColumnFails", func(t *testing.T) {
		_, err := Cursor(url.Values{"_cursor_value": {"abc"}}, &cursorAutoTestModel{})
		require.Error(t, err, "MySQL coerces a non-numeric boundary on the integer primary key to 0, silently restarting the feed")
	})

	t.Run("NumericValueOnNumericDefaultColumnPasses", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_value": {"42"}, "_cursor_next": {"true"}}, &cursorAutoTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Enabled())
		require.Equal(t, "42", cursor.Value)
	})

	t.Run("MistypedValueOnNumericColumnFails", func(t *testing.T) {
		_, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_field": {"age"},
		}, &filterTestModel{})
		require.Error(t, err, "a value the numeric cursor column cannot represent is a client error, matching filter semantics")
	})

	t.Run("MistypedValueOnTimeColumnFails", func(t *testing.T) {
		_, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_field": {"created_at"},
		}, &cursorTestModel{})
		require.Error(t, err, "a value the time cursor column cannot represent is a client error, matching filter semantics")
	})

	t.Run("MistypedValueOnBoolColumnFails", func(t *testing.T) {
		_, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_field": {"enabled"},
		}, &filterTestModel{})
		require.Error(t, err, "a value the bool cursor column cannot represent is a client error, matching filter semantics")
	})

	t.Run("MissingValueYieldsZeroCursor", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_next": {"true"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.False(t, cursor.Enabled())
	})

	t.Run("ModelWithoutCursorYieldsZeroCursor", func(t *testing.T) {
		cursor, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"created_at"},
		}, &plainTestModel{})
		require.NoError(t, err)
		require.False(t, cursor.Enabled(), "a zero cursor makes WithCursor a no-op")
	})
}

func TestParseQueryTime(t *testing.T) {
	t.Run("RFC3339UTC", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15Z")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.UTC)))
	})

	t.Run("RFC3339KeepsExplicitOffset", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15+08:00")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600))))
	})

	t.Run("RFC3339FractionalSeconds", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15.123456789Z")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 123456789, time.UTC)))
	})

	t.Run("ZoneLessValueFails", func(t *testing.T) {
		// Every zone-less spelling names a different instant per server zone,
		// so all of them are rejected: RFC 3339 with its mandatory offset is
		// the one accepted format.
		for _, value := range []string{
			"2026-07-01 08:30:15",
			"2026-07-01T08:30:15",
			"2026-07-01 08:30",
			"2026-07-01T08:30",
			"2026-07-01",
		} {
			_, err := parseQueryTime(value)
			require.Error(t, err, "zone-less value %q must be rejected", value)
		}
	})

	t.Run("UnixTimestampFails", func(t *testing.T) {
		for _, value := range []string{"1751328000", "1751328000123"} {
			_, err := parseQueryTime(value)
			require.Error(t, err, "digit-only value %q must be rejected", value)
		}
	})

	t.Run("UnsupportedFormatFails", func(t *testing.T) {
		_, err := parseQueryTime("07/01/2026")
		require.Error(t, err)
	})
}
