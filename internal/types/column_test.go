package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// nameless stands in for a virtual model: it declares no table, the way a
// model embedding model.Empty reports none.
type nameless struct{}

func (nameless) TableName() string { return "" }

func TestNewColumnReferences(t *testing.T) {
	t.Run("CarriesTheColumnName", func(t *testing.T) {
		require.Equal(t, "age", types.NewColumn[sampleTable, int]("age").Name())
	})

	t.Run("PromotesNameThroughSpecializedReferences", func(t *testing.T) {
		// NumericColumn and TimeColumn embed Column, so the accessor stays
		// available on them alongside the filter and order constructors.
		require.Equal(t, "amount", types.NewNumericColumn[sampleTable, int64]("amount").Name())
		require.Equal(t, "created_at", types.NewTimeColumn[sampleTable]("created_at").Name())
	})

	t.Run("CarriesTheTable", func(t *testing.T) {
		// The table comes from the model the reference was built for, so a
		// read can tell a column of another model apart even when the two
		// models share the column name.
		require.Equal(t, "samples", types.NewColumn[sampleTable, int]("age").Table())
		require.Equal(t, "samples", types.NewNumericColumn[sampleTable, int64]("amount").Table())
		require.Equal(t, "samples", types.NewTimeColumn[sampleTable]("created_at").Table())
	})

	// Column references are built by generated code during package
	// initialization, so an incomplete reference must not survive startup.
	t.Run("PanicsOnEmptyName", func(t *testing.T) {
		require.PanicsWithValue(t, "types: a column reference of types_test.sampleTable requires a column name",
			func() { types.NewColumn[sampleTable, string]("") })
	})

	t.Run("VirtualModelCarriesNoTable", func(t *testing.T) {
		// A virtual model has no table, and gg gen still emits its Cols for
		// the query parameters it opted in to; the references carry no table
		// and read as the plain-name constructors do.
		require.Empty(t, types.NewColumn[nameless, string]("age").Table())
		require.Empty(t, types.NewColumn[*nameless, string]("age").Table())
		require.Equal(t, types.NewFilter("", "age", types.FilterOpEq, "x"), types.NewColumn[*nameless, string]("age").Eq("x"))
	})

	t.Run("ReadsTheTableThroughAPointerModel", func(t *testing.T) {
		// Generated code names the model as a pointer, the way the rest of the
		// framework refers to it; the reference instantiates the pointee, so a
		// value-receiver TableName is reached without a nil dereference.
		require.Equal(t, "samples", types.NewColumn[*sampleTable, int]("age").Table())
	})
}

// columnSink keeps each reference a benchmark builds, so the compiler cannot
// drop the work as unused.
var columnSink types.Column[string]

// BenchmarkNewColumn measures minting a reference for a model with a typical
// footprint, the cost generic code pays each time it names its type parameter
// as the model.
func BenchmarkNewColumn(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		columnSink = types.NewColumn[*sampleRecord, string]("code")
	}
}

func TestColumnBuildsFilters(t *testing.T) {
	age := types.NewColumn[sampleTable, int]("age")
	status := types.NewColumn[sampleTable, sampleStatus]("status")
	name := types.NewColumn[sampleTable, string]("name")

	tests := []struct {
		label string
		got   types.Filter
		want  types.Filter
	}{
		{"Eq", age.Eq(18), types.NewFilter("samples", "age", types.FilterOpEq, 18)},
		{"Ne", age.Ne(18), types.NewFilter("samples", "age", types.FilterOpNe, 18)},
		{"Gt", age.Gt(18), types.NewFilter("samples", "age", types.FilterOpGt, 18)},
		{"Gte", age.Gte(18), types.NewFilter("samples", "age", types.FilterOpGte, 18)},
		{"Lt", age.Lt(18), types.NewFilter("samples", "age", types.FilterOpLt, 18)},
		{"Lte", age.Lte(18), types.NewFilter("samples", "age", types.FilterOpLte, 18)},
		{
			"In",
			status.In(sampleStatusActive, sampleStatusRemoved),
			types.NewFilter("samples", "status", types.FilterOpIn, []sampleStatus{sampleStatusActive, sampleStatusRemoved}),
		},
		{
			"NotIn",
			status.NotIn(sampleStatusRemoved),
			types.NewFilter("samples", "status", types.FilterOpNotIn, []sampleStatus{sampleStatusRemoved}),
		},
		{"Like", name.Like("sam"), types.NewFilter("samples", "name", types.FilterOpLike, "sam")},
		{"NotLike", name.NotLike("sam"), types.NewFilter("samples", "name", types.FilterOpNotLike, "sam")},
		{"StartsWith", name.StartsWith("sa"), types.NewFilter("samples", "name", types.FilterOpStartsWith, "sa")},
		{"EndsWith", name.EndsWith("le"), types.NewFilter("samples", "name", types.FilterOpEndsWith, "le")},
		{"IsNull", name.IsNull(), types.NewFilter("samples", "name", types.FilterOpIsNull, true)},
		{"IsNotNull", name.IsNotNull(), types.NewFilter("samples", "name", types.FilterOpIsNull, false)},
		{"Regex", name.Regex("^sa"), types.NewFilter("samples", "name", types.FilterOpRegex, "^sa")},
		{"NotRegex", name.NotRegex("^sa"), types.NewFilter("samples", "name", types.FilterOpNotRegex, "^sa")},
		{"JSONContains", name.JSONContains("sam"), types.NewFilter("samples", "name", types.FilterOpJSONContains, "sam")},
		{
			"EqCol",
			age.EqCol(types.NewColumn[sampleTable, int]("parent_age")),
			types.NewFilter("samples", "age", types.FilterOpEqCol, types.NewColumn[sampleTable, int]("parent_age")),
		},
		{"EqColWithoutParent", age.EqCol(nil), types.NewFilter("samples", "age", types.FilterOpEqCol, "")},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestColumnInWithoutValues(t *testing.T) {
	status := types.NewColumn[sampleTable, sampleStatus]("status")
	// A variadic call with no arguments yields a nil slice. It still carries
	// the slice type, so the database layer treats it as an empty set and
	// matches nothing rather than widening the query.
	require.Equal(t,
		types.NewFilter("samples", "status", types.FilterOpIn, []sampleStatus(nil)),
		status.In())
}

func TestColumnInKeepsItsOwnValues(t *testing.T) {
	status := types.NewColumn[sampleTable, sampleStatus]("status")
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
	category := types.NewColumn[sampleTable, string]("category")
	amount := types.NewNumericColumn[sampleTable, int64]("amount")
	occurred := types.NewTimeColumn[sampleTable]("occurred_at")
	// Every projection method yields the column with its table, aliased by
	// its own name; only the function and the bucket differ.
	term := func(fn types.TermFn, column string) types.Term {
		return types.NewTerm(fn, "samples", column, "", column)
	}
	bucket := func(b types.TimeBucket) types.Term {
		return types.NewTerm(types.FnNone, "samples", "occurred_at", b, "occurred_at")
	}

	tests := []struct {
		label string
		got   types.Term
		want  types.Term
	}{
		{"Group", category.Group(), term(types.FnNone, "category")},
		{"Count", category.Count(), term(types.FnCount, "category")},
		{"CountDistinct", category.CountDistinct(), term(types.FnCountDistinct, "category")},
		{"Min", category.Min(), term(types.FnMin, "category")},
		{"Max", category.Max(), term(types.FnMax, "category")},
		{"Lag", category.Lag(), term(types.FnLag, "category")},
		{"Lead", category.Lead(), term(types.FnLead, "category")},
		{"Sum", amount.Sum(), term(types.FnSum, "amount")},
		{"Avg", amount.Avg(), term(types.FnAvg, "amount")},
		{"ByHour", occurred.ByHour(), bucket(types.TimeBucketHour)},
		{"ByDay", occurred.ByDay(), bucket(types.TimeBucketDay)},
		{"ByMonth", occurred.ByMonth(), bucket(types.TimeBucketMonth)},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestColumnBuildsOrders(t *testing.T) {
	created := types.NewColumn[sampleTable, int]("created_at")
	require.Equal(t, types.NewOrder("samples", "created_at", types.OrderAsc), created.Asc())
	require.Equal(t, types.NewOrder("samples", "created_at", types.OrderDesc), created.Desc())
}

func TestColumnBuildsAssignments(t *testing.T) {
	// Set types the value by the column, so the assignment carries the
	// column's own value type, and the table the reference was built for.
	status := types.NewColumn[sampleTable, sampleStatus]("status")
	require.Equal(t,
		types.NewAssignment("samples", "status", sampleStatusActive),
		status.Set(sampleStatusActive))
}

func TestAnyColumnRefMixesReferenceKinds(t *testing.T) {
	// One variadic list mixes references of different Go types, which is what
	// the column-list options such as WithSelect accept.
	names := func(columns ...types.AnyColumnRef) []string {
		collected := make([]string, 0, len(columns))
		for _, column := range columns {
			collected = append(collected, column.Name())
		}
		return collected
	}
	require.Equal(t, []string{"name", "amount", "created_at"}, names(
		types.NewColumn[sampleTable, string]("name"),
		types.NewNumericColumn[sampleTable, int64]("amount"),
		types.NewTimeColumn[sampleTable]("created_at"),
	))
}
