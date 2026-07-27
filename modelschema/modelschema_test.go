package modelschema_test

import (
	"testing"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/modelschema"
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
	require.True(t, modelschema.IsQueryable(&queryRecord{}))
	require.False(t, modelschema.IsQueryable(&plainRecord{}))
}
