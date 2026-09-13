package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestNewAssignment(t *testing.T) {
	// The parts read back unchanged; an empty table is how framework code
	// names a column of the chain's own model.
	named := types.NewAssignment("samples", "status", "active")
	require.Equal(t, "samples", named.Table())
	require.Equal(t, "status", named.Column())
	require.Equal(t, "active", named.Value())

	own := types.NewAssignment("", "age", 18)
	require.Empty(t, own.Table())
	require.Equal(t, "age", own.Column())
	require.Equal(t, 18, own.Value())
}
