package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestPartitionBy(t *testing.T) {
	tenant := types.NewColumn[sampleTable, string]("tenant_id")
	occurred := types.NewTimeColumn[sampleTable]("occurred_at")

	t.Run("KeysBecomeTerms", func(t *testing.T) {
		// A column reference partitions by the column as stored and a bucket
		// by the bucket, so the window spells its keys the way the projection
		// spells them.
		window := types.PartitionBy(tenant, occurred.ByDay())
		require.Equal(t, []types.Term{types.TermOf(tenant), occurred.ByDay()}, types.WindowPartitionOf(window))
		require.Empty(t, types.WindowOrdersOf(window))
	})

	t.Run("WithoutKeysIsOnePartition", func(t *testing.T) {
		require.Empty(t, types.WindowPartitionOf(types.PartitionBy()))
	})

	t.Run("WithoutKeysIsTheOrderedWindow", func(t *testing.T) {
		// The two spellings build one value, so a term declared through one
		// is found by a condition or an ordering written through the other.
		total := types.NewNumericColumn[sampleTable, int64]("amount").Sum()
		require.Equal(t, types.OrderBy(total.Desc()), types.PartitionBy().OrderBy(total.Desc()))
	})
}

func TestWindowOrderBy(t *testing.T) {
	created := types.NewColumn[sampleTable, int]("created_at")
	total := types.NewNumericColumn[sampleTable, int64]("amount").Sum()

	t.Run("OpensAnUnpartitionedWindow", func(t *testing.T) {
		window := types.OrderBy(total.Desc())
		require.Empty(t, types.WindowPartitionOf(window))
		require.Equal(t, []types.Ordering{total.Desc()}, types.WindowOrdersOf(window))
	})

	t.Run("CopiesTheOrders", func(t *testing.T) {
		// The window keeps its own copy: a caller rewriting the slice it
		// passed does not rewrite the window.
		orders := []types.Ordering{total.Desc()}
		window := types.OrderBy(orders...)
		orders[0] = total.Asc()
		require.Equal(t, []types.Ordering{total.Desc()}, types.WindowOrdersOf(window))
	})

	t.Run("AppendsToThePartitionedWindow", func(t *testing.T) {
		window := types.PartitionBy(created).OrderBy(created.Desc(), total.Asc())
		require.Equal(t, []types.Term{types.TermOf(created)}, types.WindowPartitionOf(window))
		require.Equal(t, []types.Ordering{created.Desc(), total.Asc()}, types.WindowOrdersOf(window))
	})
}
