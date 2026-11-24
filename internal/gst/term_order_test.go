package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestTermBuildsOrders(t *testing.T) {
	total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum()
	require.Equal(t, gst.NewTermOrder(total, gst.OrderAsc), total.Asc())
	require.Equal(t, gst.NewTermOrder(total, gst.OrderDesc), total.Desc())
}
