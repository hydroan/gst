package types_test

import (
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestCursorPositionConstructors(t *testing.T) {
	forward := types.CursorForward(types.Asc("id"), "abc")
	require.Equal(t, types.Order{Column: "id", Direction: types.OrderAsc}, forward.Order)
	require.Equal(t, "abc", forward.Value)
	require.False(t, forward.Backward)

	backward := types.CursorBackward(types.Desc("created_at"), "abc")
	require.Equal(t, types.Order{Column: "created_at", Direction: types.OrderDesc}, backward.Order)
	require.True(t, backward.Backward)
}

func TestCursorPositionEnabled(t *testing.T) {
	require.False(t, types.CursorPosition{}.Enabled(), "a zero cursor makes WithCursor a no-op")
	require.False(t, types.CursorForward(types.Asc("id"), "").Enabled(), "a cursor without a boundary value is not enabled")
	require.True(t, types.CursorForward(types.Asc("id"), "abc").Enabled())
}
