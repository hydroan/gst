package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestJoinSources(t *testing.T) {
	on := gst.NewColumn[*sampleRecord, string]("code").EqCol(gst.NewColumn[sampleTable, string]("record_code"))

	t.Run("JoinAllocatesTheModelAndKeepsTheOn", func(t *testing.T) {
		source, ok := gst.Join[*sampleRecord](on).(gst.ModelJoin)
		require.True(t, ok)
		require.IsType(t, &sampleRecord{}, source.Model, "the model is allocated, so the database layer reads its table and columns without a type parameter")
		require.False(t, source.Left)
		require.Equal(t, []gst.Filter{on}, source.On)
	})

	t.Run("ConstructorsKeepTheirOwnOn", func(t *testing.T) {
		// Two joins built from one prefix slice with spare capacity keep
		// their own predicates: the second append does not rewrite the first.
		base := make([]gst.Filter, 0, 4)
		base = append(base, on)
		gold, ok := gst.Join[*sampleRecord](append(base, gst.FilterEq("tier", "gold"))...).(gst.ModelJoin)
		require.True(t, ok)
		silver, ok := gst.Join[*sampleRecord](append(base, gst.FilterEq("tier", "silver"))...).(gst.ModelJoin)
		require.True(t, ok)
		require.Equal(t, "gold", gold.On[1].Value())
		require.Equal(t, "silver", silver.On[1].Value())
		goldSelect, ok := gst.JoinSelect[struct{ Code string }](stubBranch{}, append(base, gst.FilterEq("tier", "gold"))...).(gst.SelectJoin)
		require.True(t, ok)
		silverSelect, ok := gst.LeftJoinSelect[struct{ Code string }](stubBranch{}, append(base, gst.FilterEq("tier", "silver"))...).(gst.SelectJoin)
		require.True(t, ok)
		require.Equal(t, "gold", goldSelect.On[1].Value())
		require.Equal(t, "silver", silverSelect.On[1].Value())
	})

	t.Run("LeftJoinKeepsUnmatchedRows", func(t *testing.T) {
		source, ok := gst.LeftJoin[*sampleRecord](on).(gst.ModelJoin)
		require.True(t, ok)
		require.True(t, source.Left)
	})

	t.Run("EqColCarriesBothTables", func(t *testing.T) {
		// A join predicate is placed by the tables its two columns carry, so
		// the reference travels whole rather than as a name.
		require.Equal(t, "sample_records", on.Table())
		parent, ok := on.Value().(gst.AnyColumnRef)
		require.True(t, ok)
		require.Equal(t, "samples", parent.Table())
		require.Equal(t, "record_code", parent.Name())
	})
}

func TestJoinSelectSources(t *testing.T) {
	on := gst.NewColumn[*sampleRecord, string]("code").EqCol(gst.NewColumn[sampleTable, string]("record_code"))
	// Any SelectBranch serves as the joined select at this level; the
	// database layer checks that it is one of its own selects.
	sub := stubBranch{}

	t.Run("JoinSelectKeepsTheSelectAndTheOn", func(t *testing.T) {
		source, ok := gst.JoinSelect[struct{ Code string }](sub, on).(gst.SelectJoin)
		require.True(t, ok)
		require.Equal(t, sub, source.Select)
		require.False(t, source.Left)
		require.Equal(t, []gst.Filter{on}, source.On)
	})

	t.Run("LeftJoinSelectKeepsUnmatchedRows", func(t *testing.T) {
		source, ok := gst.LeftJoinSelect[struct{ Code string }](sub, on).(gst.SelectJoin)
		require.True(t, ok)
		require.True(t, source.Left)
	})
}

// stubBranch stands in for a select: SelectBranch is structural, so a value
// with the two terminals fills the role at compile time.
type stubBranch struct{}

func (stubBranch) Scan(*[]struct{ Code string }) error { return nil }
func (stubBranch) Count(*int) error                    { return nil }
