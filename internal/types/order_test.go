package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestOrderConstructors(t *testing.T) {
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderAsc}, types.Asc("created_at"))
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderDesc}, types.Desc("created_at"))
}

func TestOrderSortsBy(t *testing.T) {
	createdAt := types.NewTimeColumn[sampleTable]("created_at")

	require.True(t, types.Order{Column: "created_at", Direction: types.OrderDesc}.SortsBy(createdAt),
		"an order parsed from a request names no table and matches by column name")
	require.True(t, createdAt.Asc().SortsBy(createdAt))
	require.False(t, types.NewTimeColumn[*sampleRecord]("created_at").Asc().SortsBy(createdAt),
		"an order built for another table does not sort by this column even when the names agree")
	require.False(t, types.Asc("updated_at").SortsBy(createdAt))
}

func TestOrderDescending(t *testing.T) {
	require.True(t, types.Desc("created_at").Descending())
	require.False(t, types.Asc("created_at").Descending())
	require.False(t, types.Order{Column: "created_at"}.Descending(), "the zero direction sorts ascending")
}

func TestOrderDirectionValid(t *testing.T) {
	require.True(t, types.OrderAsc.Valid())
	require.True(t, types.OrderDesc.Valid())
	require.True(t, types.OrderDirection("").Valid(), "the zero direction reads as ascending")
	require.False(t, types.OrderDirection("asc").Valid(), "a direction is spelled the way SQL spells it")
}

func TestOrderDirectionFlip(t *testing.T) {
	require.Equal(t, types.OrderDesc, types.OrderAsc.Flip())
	require.Equal(t, types.OrderAsc, types.OrderDesc.Flip())
	require.Equal(t, types.OrderDesc, types.OrderDirection("").Flip(), "a zero direction means ascending, so it flips to descending")
}
