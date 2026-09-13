package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowOrderByKeepsDerivedWindowsApart(t *testing.T) {
	created := Desc("created_at")
	total := Term{fn: FnSum, table: "samples", column: "amount", alias: "amount"}
	// Spare capacity on the orders is where a shared backing array would show:
	// two windows derived from one base must not overwrite each other.
	base := Window{orders: append(make([]Ordering, 0, 4), created)}
	byTotalAsc := base.OrderBy(total.Asc())
	byTotalDesc := base.OrderBy(total.Desc())
	require.Equal(t, []Ordering{created, total.Asc()}, byTotalAsc.orders)
	require.Equal(t, []Ordering{created, total.Desc()}, byTotalDesc.orders)
	require.Equal(t, []Ordering{created}, base.orders, "the base window is left as it was")
}
