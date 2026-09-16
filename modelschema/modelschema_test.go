package modelschema_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/modelschema"
	"github.com/stretchr/testify/require"
)

// queryRecord opts in to framework query parameters; plainRecord does not.
type queryRecord struct {
	modelregistry.Query
	modelregistry.Empty
}

type plainRecord struct {
	modelregistry.Empty
}

func TestIsQueryable(t *testing.T) {
	require.True(t, modelschema.IsQueryable(&queryRecord{}))
	require.False(t, modelschema.IsQueryable(&plainRecord{}))
}
