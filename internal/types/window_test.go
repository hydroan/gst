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
		require.Equal(t, []types.Term{types.TermOf(tenant), occurred.ByDay()}, window.Partition)
		require.Empty(t, window.Orders)
	})

	t.Run("WithoutKeysIsOnePartition", func(t *testing.T) {
		require.Empty(t, types.PartitionBy().Partition)
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
		require.Empty(t, window.Partition)
		require.Equal(t, []types.Ordering{total.Desc()}, window.Orders)
	})

	t.Run("CopiesTheOrders", func(t *testing.T) {
		// The window keeps its own copy: a caller rewriting the slice it
		// passed does not rewrite the window.
		orders := []types.Ordering{total.Desc()}
		window := types.OrderBy(orders...)
		orders[0] = total.Asc()
		require.Equal(t, []types.Ordering{total.Desc()}, window.Orders)
	})

	t.Run("AppendsToThePartitionedWindow", func(t *testing.T) {
		window := types.PartitionBy(created).OrderBy(created.Desc(), total.Asc())
		require.Equal(t, []types.Term{types.TermOf(created)}, window.Partition)
		require.Equal(t, []types.Ordering{created.Desc(), total.Asc()}, window.Orders)
	})

	t.Run("ExtendingTwiceKeepsBothWindows", func(t *testing.T) {
		// Spare capacity on the orders is where a shared backing array would
		// show: two windows derived from one base must not overwrite each
		// other.
		base := types.Window{Orders: append(make([]types.Ordering, 0, 4), created.Desc())}
		byTotalAsc := base.OrderBy(total.Asc())
		byTotalDesc := base.OrderBy(total.Desc())
		require.Equal(t, []types.Ordering{created.Desc(), total.Asc()}, byTotalAsc.Orders)
		require.Equal(t, []types.Ordering{created.Desc(), total.Desc()}, byTotalDesc.Orders)
		require.Equal(t, []types.Ordering{created.Desc()}, base.Orders, "the base window is left as it was")
	})
}
