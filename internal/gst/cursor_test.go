package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestCursorConstructors(t *testing.T) {
	forward := gst.CursorForward(gst.Asc("id"), "abc")
	require.Equal(t, gst.NewOrder("", "id", gst.OrderAsc), forward.Order())
	require.Equal(t, "abc", forward.Value())
	require.False(t, forward.Backward())

	backward := gst.CursorBackward(gst.Desc("created_at"), "abc")
	require.Equal(t, gst.NewOrder("", "created_at", gst.OrderDesc), backward.Order())
	require.True(t, backward.Backward())
}

func TestCursorEnabled(t *testing.T) {
	require.False(t, gst.Cursor{}.Enabled(), "a zero cursor makes WithCursor a no-op")
	first := gst.CursorForward(gst.Asc("id"), "")
	require.True(t, first.Enabled(), "a cursor without a boundary value is the feed's first page, ordered by its column")
	require.False(t, first.Bounded(), "the first page has no boundary to start past")
	next := gst.CursorForward(gst.Asc("id"), "abc")
	require.True(t, next.Enabled())
	require.True(t, next.Bounded())
}
