package urlquery

import (
	"database/sql"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

type filterTestModel struct {
	Name      string    `query:"name"`
	Age       int       `json:"age"`
	Remark    string    `json:"remark"`
	ItemCount int       `json:"item_count"`
	Enabled   bool      `json:"enabled"`
	ExpiredAt time.Time `json:"expired_at"`
	// Day is a calendar day; ClosedAt, Retries and Archived are optional
	// values behind pointers, checked like the values they point to.
	Day      datatypes.Date `json:"day"`
	ClosedAt *time.Time     `json:"closed_at"`
	Retries  *int           `json:"retries"`
	Archived *bool          `json:"archived"`
	// GroupIDs and Renamed anchor the column resolution: gorm renders the
	// former as group_ids (a plain snake case conversion would not) and the
	// latter through its column tag, while the URL keeps the json name.
	GroupIDs string `json:"group_ids"`
	Renamed  string `json:"renamed" gorm:"column:custom_column"`
	// Secret is a column hidden from clients: json:"-" keeps it out of
	// responses and out of every query. Virtual is no column at all: a bare
	// key fills it for the service's own use.
	Secret  string `json:"-"`
	Virtual string `json:"virtual" gorm:"-"`
	// Meta is a JSON document, matched by its text and never compared.
	Meta datatypes.JSONMap `json:"meta"`

	modelregistry.Query
	modelregistry.Base
}

// gormTypedTestModel holds columns whose Go shape does not tell what gorm
// stores: Valuer wrappers gorm reads through the value they hold, a string
// its type tag declares a JSON column, and datatypes.Time, a time of day
// gorm gives the time data type of an instant.
type gormTypedTestModel struct {
	Settled sql.NullTime    `json:"settled"`
	Tally   sql.NullInt64   `json:"tally"`
	Ratio   sql.NullFloat64 `json:"ratio"`
	Active  sql.NullBool    `json:"active"`
	Payload string          `json:"payload" gorm:"type:json"`
	Clock   datatypes.Time  `json:"clock"`

	modelregistry.Query
	modelregistry.Base
}

// namedPart is embedded by name twice in namedEmbeddingTestModel, beside a
// top-level field of the same name.
type namedPart struct {
	Label string `json:"label"`
}

// namedEmbeddingTestModel holds four columns behind one field name: its own
// Label and the Label of each struct it embeds by name, Third through a
// pointer, which the URL names by the path the field takes in JSON.
type namedEmbeddingTestModel struct {
	Label  string     `json:"label"`
	First  namedPart  `json:"first" gorm:"embedded;embeddedPrefix:first_"`
	Second namedPart  `json:"second" gorm:"embedded;embeddedPrefix:second_"`
	Third  *namedPart `json:"third" gorm:"embedded;embeddedPrefix:third_"`

	modelregistry.Query
	modelregistry.Base
}

// HiddenCredentials is embedded by embeddingTestModel: its column is hidden
// from clients from inside an embedded struct, where the field's index path
// is the one gorm composes.
type HiddenCredentials struct {
	Token string `json:"-"`
}

type embeddingTestModel struct {
	HiddenCredentials
	Name string `json:"name"`

	modelregistry.Base
}

func (*embeddingTestModel) TableName() string { return "embedding_test_models" }

func TestFilters(t *testing.T) {
	t.Run("MatchesAJSONColumnByItsTextAndRefusesComparingIt", func(t *testing.T) {
		conds, err := Filters(url.Values{"meta[like]": {"alice"}, "meta[isnull]": {"true"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "meta", types.FilterOpIsNull, true),
			types.NewFilter("", "meta", types.FilterOpLike, "alice"),
		}, conds)
		for _, op := range []string{"eq", "ne", "gt", "in"} {
			_, err := Filters(url.Values{"meta[" + op + "]": {"alice"}}, &filterTestModel{})
			require.EqualError(t, err, `invalid field filter "meta[`+op+`]": operator "`+op+`" is not supported on a JSON field`,
				"a JSON document compares by rules of the dialect's own, so the comparison is refused up front")
		}
	})

	t.Run("ExtractsOperatorConditionsAndIgnoresOtherKeys", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"age[gt]":      {"20"},
			"remark[like]": {"hello"},
			"name":         {"alice"},
			"_sort_by":     {"created_at desc"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "age", types.FilterOpGt, "20"),
			types.NewFilter("", "remark", types.FilterOpLike, "hello"),
		}, conds)
	})

	t.Run("CoexistsWithExactFilterOnSameField", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"age":     {"10"},
			"age[gt]": {"20"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "age", types.FilterOpGt, "20")}, conds,
			"the bare key stays with the exact business filter and only the operator key becomes a filter")
	})

	t.Run("MapsCamelFieldToSnakeColumn", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"itemCount[notlike]": {"sample"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "item_count", types.FilterOpNotLike, "sample")}, conds)
	})

	t.Run("AcceptsBaseLiftedColumns", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"id[notin]": {"a,b"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "id", types.FilterOpNotIn, []string{"a", "b"})}, conds)
	})

	t.Run("UsesGormColumnNames", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"group_ids[in]": {"a,b"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "group_ids", types.FilterOpIn, []string{"a", "b"}),
		}, conds, "gorm renders GroupIDs as group_ids, not group_i_ds")

		conds, err = Filters(url.Values{
			"renamed[eq]": {"x"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "custom_column", types.FilterOpEq, "x"),
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
		require.EqualError(t, err, `unsupported query parameter "age[gt]"`,
			"a model that did not embed model.Query has no filterable columns, so the filter key itself is unsupported")

		_, err = Filters(url.Values{
			"age[gt]":      {"1"},
			"remark[like]": {"x"},
		}, &plainModel{})
		require.EqualError(t, err, `unsupported query parameters "age[gt]", "remark[like]"`,
			"every filter key is listed, not just the first")
	})

	t.Run("TimeFieldAcceptsOnlyRFC3339", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"expired_at[gte]": {"2026-07-01T00:00:00Z"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t, "expired_at", conds[0].Column())
		require.Equal(t,
			time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout),
			conds[0].Value(), "the bound travels as the UTC wall clock")

		conds, err = Filters(url.Values{
			"expired_at[eq]": {"2026-07-01T08:30:15+08:00"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Len(t, conds, 1)
		require.Equal(t,
			time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).UTC().Format(types.FilterTimeLayout),
			conds[0].Value(), "an explicit offset must be converted to UTC")

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

	t.Run("DateFieldKeepsTheInstant", func(t *testing.T) {
		// A date column compares by the UTC day a value falls on, which the
		// database binds; the parser keeps the instant in the canonical
		// layout, so a service reading the filter sees what was sent:
		// midnight in Shanghai on the 2nd is 16:00 on the 1st in UTC.
		conds, err := Filters(url.Values{"day[eq]": {"2026-01-02T00:00:00+08:00"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "day", types.FilterOpEq, "2026-01-01 16:00:00")}, conds)

		conds, err = Filters(url.Values{"day[gte]": {"2026-01-02T15:30:00Z"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, "2026-01-02 15:30:00", conds[0].Value(), "the time of day is kept")

		_, err = Filters(url.Values{"day[eq]": {"not-a-date"}}, &filterTestModel{})
		require.Error(t, err, "a value that is not RFC 3339 is a client error, not an empty page")
		for _, key := range []string{"day[like]", "day[in]"} {
			_, err = Filters(url.Values{key: {"2026-01-02T00:00:00Z"}}, &filterTestModel{})
			require.Error(t, err, "key %q must be rejected on a date field", key)
		}
	})

	t.Run("OptionalFieldsAreCheckedLikeTheirValues", func(t *testing.T) {
		conds, err := Filters(url.Values{"closed_at[lt]": {"2026-07-01T08:00:00+08:00"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, "2026-07-01 00:00:00", conds[0].Value(), "an optional time travels as the UTC wall clock too")

		for key, value := range map[string]string{
			"closed_at[gte]": "07/01/2026",
			"retries[gt]":    "many",
			"archived[eq]":   "maybe",
		} {
			_, err := Filters(url.Values{key: {value}}, &filterTestModel{})
			require.Error(t, err, "%s=%s must be rejected: the pointer holds a value of the column's type", key, value)
		}
	})

	t.Run("PrefixAndSuffixOpsPassStringValues", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"remark[endswith]":   {"suffix"},
			"remark[startswith]": {"prefix"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "remark", types.FilterOpEndsWith, "suffix"),
			types.NewFilter("", "remark", types.FilterOpStartsWith, "prefix"),
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
			types.NewFilter("", "expired_at", types.FilterOpIsNull, false),
			types.NewFilter("", "remark", types.FilterOpIsNull, true),
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
			types.NewFilter("", "created_at", types.FilterOpEq, time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)),
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
			types.NewFilter("", "created_at", types.FilterOpGte, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)),
			types.NewFilter("", "updated_at", types.FilterOpLt, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC).Format(types.FilterTimeLayout)),
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
		require.Equal(t, []types.Filter{types.NewFilter("", "age", types.FilterOpIn, []string{"1", "2"})}, conds)
	})

	t.Run("BoolFieldNormalizesAndGatesOps", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"enabled[eq]": {"true"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "enabled", types.FilterOpEq, true)}, conds)

		conds, err = Filters(url.Values{
			"enabled[ne]": {"0"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "enabled", types.FilterOpNe, false)}, conds)

		_, err = Filters(url.Values{
			"enabled[gt]": {"true"},
		}, &filterTestModel{})
		require.Error(t, err, "ordering operators make no sense on a bool field")

		_, err = Filters(url.Values{
			"enabled[eq]": {"yes"},
		}, &filterTestModel{})
		require.Error(t, err, "non-boolean value must be rejected")
	})

	t.Run("ReachesEmbeddedColumnsByPath", func(t *testing.T) {
		conds, err := Filters(url.Values{"label[eq]": {"l"}, "first.label[eq]": {"f"}, "second.label[like]": {"s"}}, &namedEmbeddingTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "first_label", types.FilterOpEq, "f"),
			types.NewFilter("", "label", types.FilterOpEq, "l"),
			types.NewFilter("", "second_label", types.FilterOpLike, "s"),
		}, conds, "label is the model's own column, first.label and second.label the embedded ones")
	})

	t.Run("WrappedTimeFieldAcceptsOnlyRFC3339", func(t *testing.T) {
		conds, err := Filters(url.Values{"settled[lt]": {"2026-07-01T08:00:00+08:00"}}, &gormTypedTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "settled", types.FilterOpLt, "2026-07-01 00:00:00")}, conds,
			"the time sql.NullTime holds travels as the UTC wall clock like any time")

		for key, value := range map[string]string{"settled[gt]": "1767225600", "settled[like]": "2026"} {
			_, err := Filters(url.Values{key: {value}}, &gormTypedTestModel{})
			require.Error(t, err, "%s=%s must be rejected on a time field", key, value)
		}
	})

	t.Run("WrappedNumericFieldValidatesValues", func(t *testing.T) {
		conds, err := Filters(url.Values{"tally[gt]": {"5"}, "ratio[lte]": {"0.5"}}, &gormTypedTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "ratio", types.FilterOpLte, "0.5"),
			types.NewFilter("", "tally", types.FilterOpGt, "5"),
		}, conds)

		for key, value := range map[string]string{"tally[gt]": "abc", "tally[in]": "1,x", "ratio[lt]": "half"} {
			_, err := Filters(url.Values{key: {value}}, &gormTypedTestModel{})
			require.Error(t, err, "%s=%s must be rejected on a numeric field", key, value)
		}
	})

	t.Run("WrappedBoolFieldNormalizesAndGatesOps", func(t *testing.T) {
		conds, err := Filters(url.Values{"active[eq]": {"true"}}, &gormTypedTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "active", types.FilterOpEq, true)}, conds)

		for key, value := range map[string]string{"active[eq]": "maybe", "active[gt]": "true"} {
			_, err := Filters(url.Values{key: {value}}, &gormTypedTestModel{})
			require.Error(t, err, "%s=%s must be rejected on a bool field", key, value)
		}
	})

	t.Run("TypeTaggedJSONFieldMatchesByItsTextAndRefusesComparingIt", func(t *testing.T) {
		conds, err := Filters(url.Values{"payload[like]": {"alice"}}, &gormTypedTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{types.NewFilter("", "payload", types.FilterOpLike, "alice")}, conds)

		_, err = Filters(url.Values{"payload[eq]": {"alice"}}, &gormTypedTestModel{})
		require.EqualError(t, err, `invalid field filter "payload[eq]": operator "eq" is not supported on a JSON field`)
	})

	t.Run("TimeOfDayFieldAcceptsOnlyATimeOfDay", func(t *testing.T) {
		conds, err := Filters(url.Values{"clock[gte]": {"09:00:00"}, "clock[lt]": {"17:30:00.5"}}, &gormTypedTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Filter{
			types.NewFilter("", "clock", types.FilterOpGte, "09:00:00"),
			types.NewFilter("", "clock", types.FilterOpLt, "17:30:00.500000000"),
		}, conds, "the bound is the spelling datatypes.Time stores")

		for key, value := range map[string]string{
			"clock[gt]":   "2026-01-01T09:00:00Z",
			"clock[eq]":   "123000",
			"clock[like]": "09",
			"clock[in]":   "09:00:00",
		} {
			_, err := Filters(url.Values{key: {value}}, &gormTypedTestModel{})
			require.Error(t, err, "%s=%s must be rejected on a time of day field", key, value)
		}
	})

	t.Run("LeavesFrameworkNamespaceAlone", func(t *testing.T) {
		conds, err := Filters(url.Values{
			"_page[gt]": {"1"},
		}, &filterTestModel{})
		require.NoError(t, err)
		require.Empty(t, conds, "underscore keys stay in the framework namespace and are not filters")
	})
}

func TestFilterQueryKeyOwnership(t *testing.T) {
	// The decoder steps around exactly the keys Filters owns: operator keys
	// and the bare framework timestamps. Underscore keys never count, with or
	// without brackets — a bracketed underscore key stays a framework
	// parameter and is rejected by the decoding path instead.
	for key, owned := range map[string]bool{
		"name":            false,
		"age[gt]":         true,
		"created_at":      true,
		"updated_at[lte]": true,
		"_page":           false,
		"_page[gt]":       false,
	} {
		require.Equal(t, owned, isFilterQueryKey(key), key)
	}
}

func TestDecode(t *testing.T) {
	t.Run("FillsModelFieldsByQueryTag", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"name": {"alice"}, "age": {"10"}}, &m))
		require.Equal(t, "alice", m.Name)
		require.Equal(t, 10, m.Age, "fields are mapped by the query tag, falling back to the json tag and the field name")
	})

	t.Run("FillsMultiWordJSONNamedFields", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"item_count": {"7"}, "group_ids": {"g1"}}, &m))
		require.Equal(t, 7, m.ItemCount,
			"a multi-word json name is a legal bare key; the streaming decoder silently rejected it")
		require.Equal(t, "g1", m.GroupIDs)
	})

	t.Run("FillsEmbeddedColumnsByPath", func(t *testing.T) {
		var m namedEmbeddingTestModel
		require.NoError(t, Decode(url.Values{"label": {"l"}, "first.label": {"f"}, "third.label": {"t"}}, &m))
		require.Equal(t, "l", m.Label)
		require.Equal(t, "f", m.First.Label, "a bare key names the column of a struct embedded by name by its path, as the operator keys do")
		require.Empty(t, m.Second.Label)
		require.NotNil(t, m.Third, "the struct embedded through a pointer is allocated to hold the value")
		require.Equal(t, "t", m.Third.Label)
	})

	t.Run("RejectsCaseVariantKeys", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"Age": {"1"}}, &m),
			`unsupported query parameter "Age"`,
			"query keys are contract surface: the documented snake case name is the only spelling")
	})

	t.Run("SkipsEmptyValues", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"age": {""}}, &m))
		require.Zero(t, m.Age, "an empty value means the caller is not filtering by that key")
	})

	t.Run("LastValueOfRepeatedKeyWins", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"age": {"1", "2"}}, &m))
		require.Equal(t, 2, m.Age)
	})

	t.Run("AllocatesPointerFields", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"_expand": {"related"}}, &m))
		require.NotNil(t, m.Expand)
		require.Equal(t, "related", *m.Expand)
	})

	t.Run("LeavesNonScalarFieldsUnsupported", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"expired_at": {"2026-07-01"}}, &m),
			`unsupported query parameter "expired_at"`,
			"a time column filters through created_at-style operator keys, never through a bare key")
	})

	t.Run("RejectsColumnsHiddenFromClients", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"secret": {"s"}}, &m),
			`unsupported query parameter "secret"`,
			"a column hidden from JSON is not the client's to filter, by a bare key no more than by an operator key")
		require.Empty(t, m.Secret)
	})

	t.Run("RejectsHiddenColumnsOfEmbeddedStructs", func(t *testing.T) {
		var m embeddingTestModel
		require.EqualError(t, Decode(url.Values{"token": {"t"}}, &m),
			`unsupported query parameter "token"`)
		require.NoError(t, Decode(url.Values{"name": {"n"}}, &m))
		require.Equal(t, "n", m.Name)
	})

	t.Run("FillsFieldsThatAreNotColumns", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"virtual": {"v"}}, &m))
		require.Equal(t, "v", m.Virtual, "a field that is no column is the service's parameter and decodes like any other")
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

	t.Run("DropsExportFormatKey", func(t *testing.T) {
		var m filterTestModel
		require.NoError(t, Decode(url.Values{"_format": {"csv"}, "name": {"alice"}}, &m),
			"the export format parameter belongs to the export action, not to any model field; "+
				"a virtual-resource export service parses the raw query itself and must not see it rejected")
		require.Equal(t, "alice", m.Name)
	})

	t.Run("RejectsUnknownKeys", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"bogus": {"1"}}, &m),
			`unsupported query parameter "bogus"`,
			"an unknown key is a typo and must not silently widen the result set")
		require.EqualError(t, Decode(url.Values{"_bogus": {"1"}}, &m),
			`unsupported query parameter "_bogus"`,
			"a mistyped framework parameter must be reported too")
	})

	t.Run("ListsEveryUnsupportedKey", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"bogus": {"1"}, "wrong": {""}, "_bogus": {"1"}}, &m),
			`unsupported query parameters "_bogus", "bogus", "wrong"`,
			"every offending key is reported at once and sorted, not gorilla/schema's first error plus a count")
	})

	t.Run("RejectsMistypedValues", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"age": {"abc"}}, &m),
			`invalid query parameter "age": expect a numeric value, got "abc"`)
		require.EqualError(t, Decode(url.Values{"enabled": {"maybe"}}, &m),
			`invalid query parameter "enabled": expect a boolean value, got "maybe"`)
	})

	t.Run("ReportsUnsupportedAndMistypedTogether", func(t *testing.T) {
		var m filterTestModel
		require.EqualError(t, Decode(url.Values{"bogus": {"1"}, "age": {"abc"}}, &m),
			`unsupported query parameter "bogus"; invalid query parameter "age": expect a numeric value, got "abc"`)
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
		require.Equal(t, []types.Order{types.NewOrder("", "name", types.OrderAsc)}, orders)
	})

	t.Run("DirectionIsCaseInsensitive", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"name DESC"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{types.NewOrder("", "name", types.OrderDesc)}, orders)
	})

	t.Run("MultipleTermsKeepTheirOrder", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"age desc, name asc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{
			types.NewOrder("", "age", types.OrderDesc),
			types.NewOrder("", "name", types.OrderAsc),
		}, orders)
	})

	t.Run("ColumnCarriesTheDatabaseName", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"renamed desc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{types.NewOrder("", "custom_column", types.OrderDesc)}, orders,
			"the URL names a column by its query name, but ORDER BY needs the database name")
	})

	t.Run("EmbeddedColumnSortsByPath", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"first.label desc,label"}}, &namedEmbeddingTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{
			types.NewOrder("", "first_label", types.OrderDesc),
			types.NewOrder("", "label", types.OrderAsc),
		}, orders)
	})

	t.Run("BaseTimestampIsSortable", func(t *testing.T) {
		orders, err := Orders(url.Values{"_sort_by": {"created_at desc"}}, &filterTestModel{})
		require.NoError(t, err)
		require.Equal(t, []types.Order{types.NewOrder("", "created_at", types.OrderDesc)}, orders,
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

// TestParsedConditionsReadThroughColumnReferences pins the contract between
// the parser and the column reference readers: what Filters and Orders produce
// reads back through Split, Values, ExcludedValues, Bounds and SortsBy, with
// every value in the column's own type.
func TestParsedConditionsReadThroughColumnReferences(t *testing.T) {
	query := url.Values{
		"age[in]":           {"20,30"},
		"enabled[eq]":       {"true"},
		"expired_at[gte]":   {"2026-09-01T08:00:00+08:00"},
		"expired_at[lt]":    {"2026-10-01T00:00:00Z"},
		"item_count[notin]": {"1,2"},
		"remark[ne]":        {"draft"},
		"_sort_by":          {"age desc"},
	}
	filters, err := Filters(query, &filterTestModel{})
	require.NoError(t, err)
	orders, err := Orders(query, &filterTestModel{})
	require.NoError(t, err)

	age := types.NewNumericColumn[*filterTestModel, int]("age")
	enabled := types.NewColumn[*filterTestModel, bool]("enabled")
	expiredAt := types.NewTimeColumn[*filterTestModel]("expired_at")
	itemCount := types.NewNumericColumn[*filterTestModel, int]("item_count")
	remark := types.NewColumn[*filterTestModel, string]("remark")

	own, rest := expiredAt.Split(filters)
	require.Len(t, own, 2)
	require.Len(t, rest, 4)
	lower, upper, err := expiredAt.Bounds(own)
	require.NoError(t, err)
	require.Equal(t, types.Bound[time.Time]{Value: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Inclusive: true, Present: true}, lower,
		"the parser normalized the bound to the UTC wall clock, which reads back as that instant")
	require.Equal(t, types.Bound[time.Time]{Value: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Present: true}, upper)

	ages, err := age.Values(rest)
	require.NoError(t, err)
	require.Equal(t, []int{20, 30}, ages)
	flags, err := enabled.Values(rest)
	require.NoError(t, err)
	require.Equal(t, []bool{true}, flags)

	counts, err := itemCount.ExcludedValues(rest)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, counts, "the members of a numeric notin read back in the column's own type")
	remarks, err := remark.ExcludedValues(rest)
	require.NoError(t, err)
	require.Equal(t, []string{"draft"}, remarks)

	require.True(t, orders[0].SortsBy(age))
	require.True(t, orders[0].Descending())
}

type paginatableTestModel struct {
	modelregistry.Pagination
	modelregistry.Base
}

type cursorTestModel struct {
	modelregistry.Cursor
	modelregistry.Base
}

// cursorUniqueTestModel carries the two kinds of column a cursor may page
// by besides the primary key: one under a unique index of its own, and one
// under a unique index it shares, which identifies no row on its own.
type cursorUniqueTestModel struct {
	Code       string    `json:"code" query:"code"`
	SnapshotAt time.Time `json:"snapshot_at" query:"snapshot_at"`
	Shard      string    `json:"shard" query:"shard"`
	Slot       int       `json:"slot" query:"slot"`

	modelregistry.Cursor
	modelregistry.Base
}

func (cursorUniqueTestModel) TableName() string { return "cursor_unique_test_models" }

func (cursorUniqueTestModel) Indexes() []modelschema.Index {
	return []modelschema.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"SnapshotAt"}, Unique: true},
		{Fields: []string{"Shard", "Slot"}, Unique: true},
	}
}

// cursorTypedTestModel pages by columns whose Go shape does not tell what
// gorm stores: a time held in sql.NullTime, an integer held in sql.NullInt64
// and a time of day. Each is NOT NULL under a unique index of its own, so a
// cursor may page by it.
type cursorTypedTestModel struct {
	Settled sql.NullTime   `json:"settled" gorm:"not null"`
	Tally   sql.NullInt64  `json:"tally" gorm:"not null"`
	Clock   datatypes.Time `json:"clock"`

	modelregistry.Cursor
	modelregistry.Base
}

func (cursorTypedTestModel) TableName() string { return "cursor_typed_test_models" }

func (cursorTypedTestModel) Indexes() []modelschema.Index {
	return []modelschema.Index{
		{Fields: []string{"Settled"}, Unique: true},
		{Fields: []string{"Tally"}, Unique: true},
		{Fields: []string{"Clock"}, Unique: true},
	}
}

type cursorAutoTestModel struct {
	modelregistry.Cursor
	modelregistry.AutoBase
}

type plainTestModel struct {
	modelregistry.Base
}

// queryTestModel pages both ways, by offset and by cursor, the way a model
// embedding model.Query does.
type queryTestModel struct {
	modelregistry.Query
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
		require.Equal(t, DefaultPageSize, size, "adjustable models default to a small page")

		page, _ = Pagination(url.Values{"_page": {"-5"}}, &paginatableTestModel{})
		require.Equal(t, 1, page, "a non-positive page normalizes to the first page")

		_, size = Pagination(url.Values{"_size": {"0"}}, &paginatableTestModel{})
		require.Equal(t, DefaultPageSize, size)

		_, size = Pagination(url.Values{"_size": {"101"}}, &paginatableTestModel{})
		require.Equal(t, MaxPageSize, size, "oversized page size clamps to the cap")
	})

	t.Run("UnparsableValuesFallBackToDefaults", func(t *testing.T) {
		page, size := Pagination(url.Values{"_page": {""}, "_size": {"abc"}}, &paginatableTestModel{})
		require.Equal(t, 1, page, "an unparsable page normalizes to the first page")
		require.Equal(t, DefaultPageSize, size)
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

	t.Run("FirstPageCursorResetsPage", func(t *testing.T) {
		page, _ := Pagination(url.Values{"_page": {"3"}, "_cursor_next": {"true"}}, &queryTestModel{})
		require.Equal(t, 1, page, "a cursor parameter without a value asks for the feed's first page; offset paging must not stack on it")
	})
}

func TestCursor(t *testing.T) {
	t.Run("CursorModelReadsAllThreeParameters", func(t *testing.T) {
		cursor, err := Cursor(url.Values{
			"_cursor_value": {"2026-07-01T08:30:15+08:00"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"snapshot_at"},
		}, &cursorUniqueTestModel{})
		require.NoError(t, err)
		// A time boundary is normalized like a time filter bound: the RFC 3339
		// input travels as the UTC wall clock.
		boundary := time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600)).UTC().Format(types.FilterTimeLayout)
		require.Equal(t, types.CursorForward(types.Asc("snapshot_at"), boundary), cursor)
	})

	t.Run("SharedColumnFails", func(t *testing.T) {
		// created_at is the column a feed in creation order reaches for
		// first, and the one rows share most readily: two rows written in the
		// same instant split across the page boundary, and the ones the page
		// had no room for are never read again.
		_, err := Cursor(url.Values{
			"_cursor_value": {"2026-07-01T08:30:15+08:00"},
			"_cursor_field": {"created_at"},
		}, &cursorTestModel{})
		require.ErrorContains(t, err, "not unique", "a column rows can share must be refused, not served with holes")

		// A column under a unique index it shares with another identifies no
		// row on its own either.
		_, err = Cursor(url.Values{
			"_cursor_value": {"a"},
			"_cursor_field": {"shard"},
		}, &cursorUniqueTestModel{})
		require.ErrorContains(t, err, "not unique")
	})

	t.Run("ColumnWithItsOwnUniqueIndexPasses", func(t *testing.T) {
		cursor, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"code"},
		}, &cursorUniqueTestModel{})
		require.NoError(t, err, "a column no two rows share leaves no gap between pages")
		require.Equal(t, "code", cursor.Order().Column())
	})

	t.Run("BoundaryFollowsTheTypeGormStores", func(t *testing.T) {
		cursor := func(field, value string) (types.Cursor, error) {
			return Cursor(url.Values{"_cursor_value": {value}, "_cursor_next": {"true"}, "_cursor_field": {field}}, &cursorTypedTestModel{})
		}
		got, err := cursor("settled", "2026-07-01T08:00:00+08:00")
		require.NoError(t, err)
		require.Equal(t, types.CursorForward(types.Asc("settled"), "2026-07-01 00:00:00"), got,
			"the time sql.NullTime holds is normalized like any time boundary")

		got, err = cursor("clock", "17:30:00.5")
		require.NoError(t, err)
		require.Equal(t, types.CursorForward(types.Asc("clock"), "17:30:00.500000000"), got,
			"a time of day travels in the spelling datatypes.Time stores")

		for field, value := range map[string]string{"settled": "1767225600", "tally": "abc", "clock": "123000"} {
			_, err := cursor(field, value)
			require.Error(t, err, "%s cannot hold %q", field, value)
		}
	})

	t.Run("PrimaryKeyPasses", func(t *testing.T) {
		cursor, err := Cursor(url.Values{
			"_cursor_value": {"abc"},
			"_cursor_next":  {"true"},
			"_cursor_field": {"id"},
		}, &cursorTestModel{})
		require.NoError(t, err)
		require.Equal(t, "id", cursor.Order().Column())
	})

	t.Run("MissingDirectionTravelsBackward", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_value": {"abc"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Backward())
		require.Empty(t, cursor.Order().Column(), "an unnamed column leaves the primary key fallback to the database layer")
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
		require.Equal(t, "42", cursor.Value())
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

	t.Run("MissingValueIsTheFirstPage", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_next": {"true"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Enabled(), "a cursor model's feed is ordered from its first page on")
		require.False(t, cursor.Bounded(), "the first page has no boundary to start past")
		require.False(t, cursor.Backward())
		require.Empty(t, cursor.Order().Column(), "an unnamed column leaves the primary key fallback to the database layer")
	})

	t.Run("MissingValueKeepsTheNamedColumn", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_field": {"code"}}, &cursorUniqueTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Enabled())
		require.False(t, cursor.Bounded())
		require.Equal(t, "code", cursor.Order().Column(), "the first page is ordered by the column the feed will page by")
	})

	t.Run("UnknownColumnFailsOnTheFirstPage", func(t *testing.T) {
		_, err := Cursor(url.Values{"_cursor_field": {"no_such_column"}}, &cursorTestModel{})
		require.Error(t, err, "a mistyped cursor column is refused on the first page, not on the second")
	})

	t.Run("SortByWithoutValueYieldsZeroCursor", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_sort_by": {"name"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.False(t, cursor.Enabled(), "a list sorted by the client is not a feed; the cursor orders nothing")
	})

	t.Run("PlainRequestOnAnOffsetPagedModelIsNotAFeed", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_size": {"2"}}, &queryTestModel{})
		require.NoError(t, err)
		require.False(t, cursor.Enabled(), "a model paging by offset too reads its first page by offset unless a cursor parameter asks for the feed")
	})

	t.Run("CursorParameterWithoutValueIsTheFirstPageOfAnOffsetPagedModel", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_cursor_next": {"true"}}, &queryTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Enabled())
		require.False(t, cursor.Bounded())
	})

	t.Run("CursorOnlyModelReadsItsFirstPageFromAPlainRequest", func(t *testing.T) {
		cursor, err := Cursor(url.Values{"_size": {"2"}}, &cursorTestModel{})
		require.NoError(t, err)
		require.True(t, cursor.Enabled(), "a model with no other paging has nothing but the feed")
		require.False(t, cursor.Bounded())
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

// TestNormalizeCursorValueFollowsTheColumnType pins the boundary check on
// the value the column holds, a pointer dereferenced, a date column's
// boundary kept as the instant sent, which the database compares by its UTC
// day; the zero Column, a model without the fallback column, passes the
// value through.
func TestNormalizeCursorValueFollowsTheColumnType(t *testing.T) {
	columns, err := modelschema.Columns(reflect.TypeFor[filterTestModel]())
	require.NoError(t, err)
	column := func(name string) modelschema.Column {
		t.Helper()
		for _, c := range columns {
			if c.DBName == name {
				return c
			}
		}
		t.Fatalf("filterTestModel has no column %q", name)
		return modelschema.Column{}
	}

	value, err := normalizeCursorValue(column("day"), "2026-01-02T00:00:00+08:00")
	require.NoError(t, err)
	require.Equal(t, "2026-01-01 16:00:00", value, "a date boundary keeps the instant")

	value, err = normalizeCursorValue(column("closed_at"), "2026-01-02T08:00:00+08:00")
	require.NoError(t, err)
	require.Equal(t, "2026-01-02 00:00:00", value, "an optional time boundary is the UTC wall clock")

	for _, name := range []string{"closed_at", "retries", "archived"} {
		_, err = normalizeCursorValue(column(name), "abc")
		require.Error(t, err, "%s cannot hold %q", name, "abc")
	}

	value, err = normalizeCursorValue(modelschema.Column{}, "abc")
	require.NoError(t, err)
	require.Equal(t, "abc", value)
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
