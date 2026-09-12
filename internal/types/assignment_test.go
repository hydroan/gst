package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestAssign(t *testing.T) {
	// Assign takes a plain column name for code that cannot reference a
	// generated column, mirroring the FilterXxx and Asc/Desc constructors.
	require.Equal(t,
		types.Assignment{Column: "age", Value: 18},
		types.Assign("age", 18))
}
