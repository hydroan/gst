package types_test

import (
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// sampleStatus is a named string type, standing in for a model enum.
type sampleStatus string

const (
	sampleStatusActive  sampleStatus = "active"
	sampleStatusRemoved sampleStatus = "removed"
)

// sampleTable stands in for the model a column reference is generated for;
// only its table name takes part.
type sampleTable struct{}

func (sampleTable) TableName() string { return "samples" }

// nameless is a model that declares no table, which a reference must refuse.
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
		require.Panics(t, func() { types.NewColumn[sampleTable, string]("") })
	})

	t.Run("PanicsOnEmptyTable", func(t *testing.T) {
		require.Panics(t, func() { types.NewColumn[nameless, string]("age") })
		require.Panics(t, func() { types.NewColumn[*nameless, string]("age") })
	})

	t.Run("ReadsTheTableThroughAPointerModel", func(t *testing.T) {
		// Generated code names the model as a pointer, the way the rest of the
		// framework refers to it; the reference instantiates the pointee, so a
		// value-receiver TableName is reached without a nil dereference.
		require.Equal(t, "samples", types.NewColumn[*sampleTable, int]("age").Table())
	})
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
		{"Eq", age.Eq(18), types.Filter{Column: "age", Op: types.FilterOpEq, Value: 18}},
		{"Ne", age.Ne(18), types.Filter{Column: "age", Op: types.FilterOpNe, Value: 18}},
		{"Gt", age.Gt(18), types.Filter{Column: "age", Op: types.FilterOpGt, Value: 18}},
		{"Gte", age.Gte(18), types.Filter{Column: "age", Op: types.FilterOpGte, Value: 18}},
		{"Lt", age.Lt(18), types.Filter{Column: "age", Op: types.FilterOpLt, Value: 18}},
		{"Lte", age.Lte(18), types.Filter{Column: "age", Op: types.FilterOpLte, Value: 18}},
		{
			"In",
			status.In(sampleStatusActive, sampleStatusRemoved),
			types.Filter{Column: "status", Op: types.FilterOpIn, Value: []sampleStatus{sampleStatusActive, sampleStatusRemoved}},
		},
		{
			"NotIn",
			status.NotIn(sampleStatusRemoved),
			types.Filter{Column: "status", Op: types.FilterOpNotIn, Value: []sampleStatus{sampleStatusRemoved}},
		},
		{"Like", name.Like("sam"), types.Filter{Column: "name", Op: types.FilterOpLike, Value: "sam"}},
		{"NotLike", name.NotLike("sam"), types.Filter{Column: "name", Op: types.FilterOpNotLike, Value: "sam"}},
		{"StartsWith", name.StartsWith("sa"), types.Filter{Column: "name", Op: types.FilterOpStartsWith, Value: "sa"}},
		{"EndsWith", name.EndsWith("le"), types.Filter{Column: "name", Op: types.FilterOpEndsWith, Value: "le"}},
		{"IsNull", name.IsNull(), types.Filter{Column: "name", Op: types.FilterOpIsNull, Value: true}},
		{"IsNotNull", name.IsNotNull(), types.Filter{Column: "name", Op: types.FilterOpIsNull, Value: false}},
		{"Regex", name.Regex("^sa"), types.Filter{Column: "name", Op: types.FilterOpRegex, Value: "^sa"}},
		{"NotRegex", name.NotRegex("^sa"), types.Filter{Column: "name", Op: types.FilterOpNotRegex, Value: "^sa"}},
		{"JSONContains", name.JSONContains("sam"), types.Filter{Column: "name", Op: types.FilterOpJSONContains, Value: "sam"}},
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
		types.Filter{Column: "status", Op: types.FilterOpIn, Value: []sampleStatus(nil)},
		status.In())
}

func TestColumnBuildsOrders(t *testing.T) {
	created := types.NewColumn[sampleTable, int]("created_at")
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderAsc}, created.Asc())
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderDesc}, created.Desc())
}

func TestColumnBuildsAssignments(t *testing.T) {
	t.Run("SetTypesTheValueByTheColumn", func(t *testing.T) {
		status := types.NewColumn[sampleTable, sampleStatus]("status")
		require.Equal(t,
			types.Assignment{Column: "status", Value: sampleStatusActive},
			status.Set(sampleStatusActive))
	})

	t.Run("AssignIsTheDynamicColumnEscapeHatch", func(t *testing.T) {
		// Assign takes a plain column name for code that cannot reference a
		// generated column, mirroring the FilterXxx and Asc/Desc constructors.
		require.Equal(t,
			types.Assignment{Column: "age", Value: 18},
			types.Assign("age", 18))
	})
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
