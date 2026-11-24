package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestPartitionBy(t *testing.T) {
	tenant := gst.NewColumn[sampleTable, string]("tenant_id")
	occurred := gst.NewTimeColumn[sampleTable]("occurred_at")

	t.Run("KeysBecomeTerms", func(t *testing.T) {
		// A column reference partitions by the column as stored and a bucket
		// by the bucket, so the window spells its keys the way the projection
		// spells them.
		window := gst.PartitionBy(tenant, occurred.ByDay())
		require.Equal(t, []gst.Term{gst.TermOf(tenant), occurred.ByDay()}, gst.WindowPartitionOf(window))
		require.Empty(t, gst.WindowOrdersOf(window))
	})

	t.Run("WithoutKeysIsOnePartition", func(t *testing.T) {
		require.Empty(t, gst.WindowPartitionOf(gst.PartitionBy()))
	})

	t.Run("WithoutKeysIsTheOrderedWindow", func(t *testing.T) {
		// The two spellings build one value, so a term declared through one
		// is found by a condition or an ordering written through the other.
		total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum()
		require.Equal(t, gst.OrderBy(total.Desc()), gst.PartitionBy().OrderBy(total.Desc()))
	})
}

func TestWindowOrderBy(t *testing.T) {
	created := gst.NewColumn[sampleTable, int]("created_at")
	total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum()

	t.Run("OpensAnUnpartitionedWindow", func(t *testing.T) {
		window := gst.OrderBy(total.Desc())
		require.Empty(t, gst.WindowPartitionOf(window))
		require.Equal(t, []gst.Ordering{total.Desc()}, gst.WindowOrdersOf(window))
	})

	t.Run("CopiesTheOrders", func(t *testing.T) {
		// The window keeps its own copy: a caller rewriting the slice it
		// passed does not rewrite the window.
		orders := []gst.Ordering{total.Desc()}
		window := gst.OrderBy(orders...)
		orders[0] = total.Asc()
		require.Equal(t, []gst.Ordering{total.Desc()}, gst.WindowOrdersOf(window))
	})

	t.Run("AppendsToThePartitionedWindow", func(t *testing.T) {
		window := gst.PartitionBy(created).OrderBy(created.Desc(), total.Asc())
		require.Equal(t, []gst.Term{gst.TermOf(created)}, gst.WindowPartitionOf(window))
		require.Equal(t, []gst.Ordering{created.Desc(), total.Asc()}, gst.WindowOrdersOf(window))
	})
}
