package gst_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

// nameless stands in for a virtual model: it declares no table, the way a
// model embedding model.Empty reports none.
type nameless struct{}

func (nameless) TableName() string { return "" }

func TestNewColumnReferences(t *testing.T) {
	t.Run("CarriesTheColumnName", func(t *testing.T) {
		require.Equal(t, "age", gst.NewColumn[sampleTable, int]("age").Name())
	})

	t.Run("PromotesNameThroughSpecializedReferences", func(t *testing.T) {
		// NumericColumn and TimeColumn embed Column, so the accessor stays
		// available on them alongside the filter and order constructors.
		require.Equal(t, "amount", gst.NewNumericColumn[sampleTable, int64]("amount").Name())
		require.Equal(t, "created_at", gst.NewTimeColumn[sampleTable]("created_at").Name())
		require.Equal(t, "tags", gst.NewJSONStringsColumn[sampleTable]("tags").Name())
	})

	t.Run("CarriesTheTable", func(t *testing.T) {
		// The table comes from the model the reference was built for, so a
		// read can tell a column of another model apart even when the two
		// models share the column name.
		require.Equal(t, "samples", gst.NewColumn[sampleTable, int]("age").Table())
		require.Equal(t, "samples", gst.NewNumericColumn[sampleTable, int64]("amount").Table())
		require.Equal(t, "samples", gst.NewTimeColumn[sampleTable]("created_at").Table())
		require.Equal(t, "samples", gst.NewJSONStringsColumn[sampleTable]("tags").Table())
	})

	// Column references are built by generated code during package
	// initialization, so an incomplete reference must not survive startup.
	t.Run("PanicsOnEmptyName", func(t *testing.T) {
		require.PanicsWithValue(t, "gst: a column reference of gst_test.sampleTable requires a column name",
			func() { gst.NewColumn[sampleTable, string]("") })
	})

	t.Run("VirtualModelCarriesNoTable", func(t *testing.T) {
		// A virtual model has no table, and gg gen still emits its Cols for
		// the query parameters it opted in to; the references carry no table
		// and read as the plain-name constructors do.
		require.Empty(t, gst.NewColumn[nameless, string]("age").Table())
		require.Empty(t, gst.NewColumn[*nameless, string]("age").Table())
		require.Equal(t, gst.NewFilter("", "age", gst.FilterOpEq, "x"), gst.NewColumn[*nameless, string]("age").Eq("x"))
	})

	t.Run("ReadsTheTableThroughAPointerModel", func(t *testing.T) {
		// Generated code names the model as a pointer, the way the rest of the
		// framework refers to it; the reference instantiates the pointee, so a
		// value-receiver TableName is reached without a nil dereference.
		require.Equal(t, "samples", gst.NewColumn[*sampleTable, int]("age").Table())
	})
}

// columnSink keeps each reference a benchmark builds, so the compiler cannot
// drop the work as unused.
var columnSink gst.Column[string]

// BenchmarkNewColumn measures minting a reference for a model with a typical
// footprint, the cost generic code pays each time it names its type parameter
// as the model.
func BenchmarkNewColumn(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		columnSink = gst.NewColumn[*sampleRecord, string]("code")
	}
}

func TestColumnBuildsFilters(t *testing.T) {
	age := gst.NewColumn[sampleTable, int]("age")
	status := gst.NewColumn[sampleTable, sampleStatus]("status")
	name := gst.NewColumn[sampleTable, string]("name")

	tests := []struct {
		label string
		got   gst.Filter
		want  gst.Filter
	}{
		{"Eq", age.Eq(18), gst.NewFilter("samples", "age", gst.FilterOpEq, 18)},
		{"Ne", age.Ne(18), gst.NewFilter("samples", "age", gst.FilterOpNe, 18)},
		{"Gt", age.Gt(18), gst.NewFilter("samples", "age", gst.FilterOpGt, 18)},
		{"Gte", age.Gte(18), gst.NewFilter("samples", "age", gst.FilterOpGte, 18)},
		{"Lt", age.Lt(18), gst.NewFilter("samples", "age", gst.FilterOpLt, 18)},
		{"Lte", age.Lte(18), gst.NewFilter("samples", "age", gst.FilterOpLte, 18)},
		{
			"In",
			status.In(sampleStatusActive, sampleStatusRemoved),
			gst.NewFilter("samples", "status", gst.FilterOpIn, []sampleStatus{sampleStatusActive, sampleStatusRemoved}),
		},
		{
			"NotIn",
			status.NotIn(sampleStatusRemoved),
			gst.NewFilter("samples", "status", gst.FilterOpNotIn, []sampleStatus{sampleStatusRemoved}),
		},
		{"Like", name.Like("sam"), gst.NewFilter("samples", "name", gst.FilterOpLike, "sam")},
		{"NotLike", name.NotLike("sam"), gst.NewFilter("samples", "name", gst.FilterOpNotLike, "sam")},
		{"ILike", name.ILike("sam"), gst.NewFilter("samples", "name", gst.FilterOpILike, "sam")},
		{"StartsWith", name.StartsWith("sa"), gst.NewFilter("samples", "name", gst.FilterOpStartsWith, "sa")},
		{"EndsWith", name.EndsWith("le"), gst.NewFilter("samples", "name", gst.FilterOpEndsWith, "le")},
		{"IsNull", name.IsNull(), gst.NewFilter("samples", "name", gst.FilterOpIsNull, true)},
		{"IsNotNull", name.IsNotNull(), gst.NewFilter("samples", "name", gst.FilterOpIsNull, false)},
		{"Regex", name.Regex("^sa"), gst.NewFilter("samples", "name", gst.FilterOpRegex, "^sa")},
		{"NotRegex", name.NotRegex("^sa"), gst.NewFilter("samples", "name", gst.FilterOpNotRegex, "^sa")},
		{
			"EqCol",
			age.EqCol(gst.NewColumn[sampleTable, int]("parent_age")),
			gst.NewFilter("samples", "age", gst.FilterOpEqCol, gst.NewColumn[sampleTable, int]("parent_age")),
		},
		{"EqColWithoutParent", age.EqCol(nil), gst.NewFilter("samples", "age", gst.FilterOpEqCol, "")},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestColumnInWithoutValues(t *testing.T) {
	status := gst.NewColumn[sampleTable, sampleStatus]("status")
	// A variadic call with no arguments yields a nil slice. It still carries
	// the slice type, so the database layer treats it as an empty set and
	// matches nothing rather than widening the query.
	require.Equal(t,
		gst.NewFilter("samples", "status", gst.FilterOpIn, []sampleStatus(nil)),
		status.In())
}

func TestColumnInKeepsItsOwnValues(t *testing.T) {
	status := gst.NewColumn[sampleTable, sampleStatus]("status")
	// Two lists built from one prefix slice with spare capacity keep their
	// own members: the second append does not rewrite the first.
	base := make([]sampleStatus, 0, 4)
	base = append(base, sampleStatusActive)
	in := status.In(append(base, sampleStatusRemoved)...)
	notIn := status.NotIn(append(base, "archived")...)
	require.Equal(t, []sampleStatus{sampleStatusActive, sampleStatusRemoved}, in.Value())
	require.Equal(t, []sampleStatus{sampleStatusActive, "archived"}, notIn.Value())
}

func TestColumnBuildsTerms(t *testing.T) {
	category := gst.NewColumn[sampleTable, string]("category")
	amount := gst.NewNumericColumn[sampleTable, int64]("amount")
	occurred := gst.NewTimeColumn[sampleTable]("occurred_at")
	day := gst.NewDateColumn[sampleTable]("day")
	// Every projection method yields the column with its table, aliased by
	// its own name; only the function and the bucket differ.
	term := func(fn gst.TermFn, column string) gst.Term {
		return gst.NewTerm(fn, "samples", column, "", column)
	}
	bucket := func(b gst.TimeBucket) gst.Term {
		return gst.NewTerm(gst.FnNone, "samples", "occurred_at", b, "occurred_at")
	}
	dayBucket := func(b gst.TimeBucket) gst.Term {
		return gst.NewTerm(gst.FnNone, "samples", "day", b, "day")
	}

	tests := []struct {
		label string
		got   gst.Term
		want  gst.Term
	}{
		{"Group", category.Group(), term(gst.FnNone, "category")},
		{"Count", category.Count(), term(gst.FnCount, "category")},
		{"CountDistinct", category.CountDistinct(), term(gst.FnCountDistinct, "category")},
		{"Min", category.Min(), term(gst.FnMin, "category")},
		{"Max", category.Max(), term(gst.FnMax, "category")},
		{"Lag", category.Lag(), term(gst.FnLag, "category")},
		{"Lead", category.Lead(), term(gst.FnLead, "category")},
		{"Sum", amount.Sum(), term(gst.FnSum, "amount")},
		{"Avg", amount.Avg(), term(gst.FnAvg, "amount")},
		{"ByHour", occurred.ByHour(), bucket(gst.TimeBucketHour)},
		{"ByDay", occurred.ByDay(), bucket(gst.TimeBucketDay)},
		{"ByMonth", occurred.ByMonth(), bucket(gst.TimeBucketMonth)},
		{"a day ByDay", day.ByDay(), dayBucket(gst.TimeBucketDay)},
		{"a day ByMonth", day.ByMonth(), dayBucket(gst.TimeBucketMonth)},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestColumnBuildsOrders(t *testing.T) {
	created := gst.NewColumn[sampleTable, int]("created_at")
	require.Equal(t, gst.NewOrder("samples", "created_at", gst.OrderAsc), created.Asc())
	require.Equal(t, gst.NewOrder("samples", "created_at", gst.OrderDesc), created.Desc())
}

func TestColumnBuildsAssignments(t *testing.T) {
	// Set types the value by the column, so the assignment carries the
	// column's own value type, and the table the reference was built for.
	status := gst.NewColumn[sampleTable, sampleStatus]("status")
	require.Equal(t,
		gst.NewAssignment("samples", "status", sampleStatusActive),
		status.Set(sampleStatusActive))
}

func TestAnyColumnRefMixesReferenceKinds(t *testing.T) {
	// One variadic list mixes references of different Go types, which is what
	// the column-list options such as WithSelect accept.
	names := func(columns ...gst.AnyColumnRef) []string {
		collected := make([]string, 0, len(columns))
		for _, column := range columns {
			collected = append(collected, column.Name())
		}
		return collected
	}
	require.Equal(t, []string{"name", "amount", "created_at"}, names(
		gst.NewColumn[sampleTable, string]("name"),
		gst.NewNumericColumn[sampleTable, int64]("amount"),
		gst.NewTimeColumn[sampleTable]("created_at"),
	))
}

// TestJSONStringsColumnBuildsTheMembershipFilter pins that the membership
// test belongs to the reference of a JSON array of strings alone: it builds
// the jsoncontains filter on the column under its table, with the member as
// the value.
func TestJSONStringsColumnBuildsTheMembershipFilter(t *testing.T) {
	f := gst.NewJSONStringsColumn[sampleTable]("tags").JSONContains("vip")
	require.Equal(t, gst.FilterOpJSONContains, f.Op())
	require.Equal(t, "tags", f.Column())
	require.Equal(t, "samples", f.Table())
	require.Equal(t, "vip", f.Value())
}

// TestJSONStringsColumnSpeaksStrings pins that the reference of a JSON array
// of strings takes and answers plain []string, naming no type of the library
// that binds the array, while every filter and assignment it builds still
// carries datatypes.JSONSlice[string], the value that binds as one JSON
// document instead of a list of parameters.
func TestJSONStringsColumnSpeaksStrings(t *testing.T) {
	tags := gst.NewJSONStringsColumn[sampleTable]("tags")

	t.Run("SignaturesNameNoLibraryType", func(t *testing.T) {
		for method := range reflect.TypeFor[gst.JSONStringsColumn]().Methods() {
			require.NotContains(t, method.Type.String(), "datatypes", method.Name)
		}
	})

	t.Run("BuildsDocuments", func(t *testing.T) {
		document := datatypes.JSONSlice[string]{"vip", "new"}
		value := []string{"vip", "new"}
		for _, f := range []gst.Filter{tags.Eq(value), tags.Ne(value), tags.Gt(value), tags.Gte(value), tags.Lt(value), tags.Lte(value)} {
			require.Equal(t, document, f.Value(), f.Op())
		}
		require.Equal(t, []datatypes.JSONSlice[string]{document}, tags.In(value).Value())
		require.Equal(t, []datatypes.JSONSlice[string]{document}, tags.NotIn(value).Value())
		require.Equal(t, document, tags.Set(value).Value())
	})

	t.Run("ReadsBackStrings", func(t *testing.T) {
		values, err := tags.Values([]gst.Filter{tags.In([]string{"vip"}, []string{"new"})})
		require.NoError(t, err)
		require.Equal(t, [][]string{{"vip"}, {"new"}}, values)
		excluded, err := tags.ExcludedValues([]gst.Filter{tags.Ne([]string{"old"})})
		require.NoError(t, err)
		require.Equal(t, [][]string{{"old"}}, excluded)
		lower, upper, err := tags.Bounds([]gst.Filter{tags.Gte([]string{"a"}), tags.Lt([]string{"z"})})
		require.NoError(t, err)
		require.Equal(t, gst.Bound[[]string]{Value: []string{"a"}, Inclusive: true, Present: true}, lower)
		require.Equal(t, gst.Bound[[]string]{Value: []string{"z"}, Present: true}, upper)
	})

	t.Run("TiesToAnotherArray", func(t *testing.T) {
		f := tags.EqCol(gst.NewJSONStringsColumn[*sampleRecord]("labels"))
		require.Equal(t, gst.FilterOpEqCol, f.Op())
		require.Equal(t, "labels", f.Value().(gst.AnyColumnRef).Name()) //nolint:errcheck // the type is the test's own
	})
}
