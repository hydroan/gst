package inspect_test

import (
	"testing"

	"github.com/hydroan/gst/inspect"
	"github.com/hydroan/gst/internal/model"
	"github.com/stretchr/testify/require"
)

// queryRecord opts in to framework query parameters; plainRecord does not.
type queryRecord struct {
	model.Query
	model.Empty
}

type plainRecord struct {
	model.Empty
}

func TestIsQueryable(t *testing.T) {
	require.True(t, inspect.IsQueryable(&queryRecord{}))
	require.False(t, inspect.IsQueryable(&plainRecord{}))
}
