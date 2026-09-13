package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTermWhereKeepsRefinementsApart(t *testing.T) {
	total := Term{fn: FnSum, table: "samples", column: "amount", alias: "amount"}
	done := FilterEq("status", "done")
	base := total.Where(done)
	// Spare capacity on the conditions is where a shared backing array would
	// show: two refinements of one term must not overwrite each other.
	base.conditions = append(make([]Filter, 0, 4), done)
	failed := base.Where(FilterEq("status", "failed"))
	vip := base.Where(FilterEq("tier", "vip"))
	require.Equal(t, []Filter{done, FilterEq("status", "failed")}, failed.conditions)
	require.Equal(t, []Filter{done, FilterEq("tier", "vip")}, vip.conditions)
	require.Empty(t, total.conditions, "the original term stays unconditional")
}
