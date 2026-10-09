package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestCursorConstructors(t *testing.T) {
	forward := types.CursorForward(types.Asc("id"), "abc")
	require.Equal(t, types.NewOrder("", "id", types.OrderAsc), forward.Order())
	require.Equal(t, "abc", forward.Value())
	require.False(t, forward.Backward())

	backward := types.CursorBackward(types.Desc("created_at"), "abc")
	require.Equal(t, types.NewOrder("", "created_at", types.OrderDesc), backward.Order())
	require.True(t, backward.Backward())
}

func TestCursorEnabled(t *testing.T) {
	require.False(t, types.Cursor{}.Enabled(), "a zero cursor makes WithCursor a no-op")
	first := types.CursorForward(types.Asc("id"), "")
	require.True(t, first.Enabled(), "a cursor without a boundary value is the feed's first page, ordered by its column")
	require.False(t, first.Bounded(), "the first page has no boundary to start past")
	next := types.CursorForward(types.Asc("id"), "abc")
	require.True(t, next.Enabled())
	require.True(t, next.Bounded())
}
