package types_test

import (
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestOrderConstructors(t *testing.T) {
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderAsc}, types.Asc("created_at"))
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderDesc}, types.Desc("created_at"))
}

func TestOrderDirectionFlip(t *testing.T) {
	require.Equal(t, types.OrderDesc, types.OrderAsc.Flip())
	require.Equal(t, types.OrderAsc, types.OrderDesc.Flip())
	require.Equal(t, types.OrderDesc, types.OrderDirection("").Flip(), "a zero direction means ascending, so it flips to descending")
}
