package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestTermBuildsOrders(t *testing.T) {
	total := types.NewNumericColumn[sampleTable, int64]("amount").Sum()
	require.Equal(t, types.NewTermOrder(total, types.OrderAsc), total.Asc())
	require.Equal(t, types.NewTermOrder(total, types.OrderDesc), total.Desc())
}
