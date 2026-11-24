package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestOrderConstructors(t *testing.T) {
	require.Equal(t, gst.NewOrder("", "created_at", gst.OrderAsc), gst.Asc("created_at"))
	require.Equal(t, gst.NewOrder("", "created_at", gst.OrderDesc), gst.Desc("created_at"))
}

func TestOrderSortsBy(t *testing.T) {
	createdAt := gst.NewTimeColumn[sampleTable]("created_at")

	require.True(t, gst.NewOrder("", "created_at", gst.OrderDesc).SortsBy(createdAt),
		"an order parsed from a request names no table and matches by column name")
	require.True(t, createdAt.Asc().SortsBy(createdAt))
	require.False(t, gst.NewTimeColumn[*sampleRecord]("created_at").Asc().SortsBy(createdAt),
		"an order built for another table does not sort by this column even when the names agree")
	require.False(t, gst.Asc("updated_at").SortsBy(createdAt))
}

func TestOrderDescending(t *testing.T) {
	require.True(t, gst.Desc("created_at").Descending())
	require.False(t, gst.Asc("created_at").Descending())
	require.False(t, gst.NewOrder("", "created_at", "").Descending(), "the zero direction sorts ascending")
}

func TestOrderDirectionValid(t *testing.T) {
	require.True(t, gst.OrderAsc.Valid())
	require.True(t, gst.OrderDesc.Valid())
	require.True(t, gst.OrderDirection("").Valid(), "the zero direction reads as ascending")
	require.False(t, gst.OrderDirection("asc").Valid(), "a direction is spelled the way SQL spells it")
}

func TestOrderDirectionFlip(t *testing.T) {
	require.Equal(t, gst.OrderDesc, gst.OrderAsc.Flip())
	require.Equal(t, gst.OrderAsc, gst.OrderDesc.Flip())
	require.Equal(t, gst.OrderDesc, gst.OrderDirection("").Flip(), "a zero direction means ascending, so it flips to descending")
}
