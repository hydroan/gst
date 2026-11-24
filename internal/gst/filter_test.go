package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestFilterConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  gst.Filter
		want gst.Filter
	}{
		{"Eq", gst.FilterEq("age", 18), gst.NewFilter("", "age", gst.FilterOpEq, 18)},
		{"Ne", gst.FilterNe("age", 18), gst.NewFilter("", "age", gst.FilterOpNe, 18)},
		{"Gt", gst.FilterGt("age", 18), gst.NewFilter("", "age", gst.FilterOpGt, 18)},
		{"Gte", gst.FilterGte("age", 18), gst.NewFilter("", "age", gst.FilterOpGte, 18)},
		{"Lt", gst.FilterLt("age", 18), gst.NewFilter("", "age", gst.FilterOpLt, 18)},
		{"Lte", gst.FilterLte("age", 18), gst.NewFilter("", "age", gst.FilterOpLte, 18)},
		{"In", gst.FilterIn("id", []string{"a", "b"}), gst.NewFilter("", "id", gst.FilterOpIn, []string{"a", "b"})},
		{"NotIn", gst.FilterNotIn("id", []int{1, 2}), gst.NewFilter("", "id", gst.FilterOpNotIn, []int{1, 2})},
		{"Like", gst.FilterLike("name", "sample"), gst.NewFilter("", "name", gst.FilterOpLike, "sample")},
		{"NotLike", gst.FilterNotLike("name", "sample"), gst.NewFilter("", "name", gst.FilterOpNotLike, "sample")},
		{"ILike", gst.FilterILike("name", "sample"), gst.NewFilter("", "name", gst.FilterOpILike, "sample")},
		{"StartsWith", gst.FilterStartsWith("name", "sam"), gst.NewFilter("", "name", gst.FilterOpStartsWith, "sam")},
		{"EndsWith", gst.FilterEndsWith("name", "ple"), gst.NewFilter("", "name", gst.FilterOpEndsWith, "ple")},
		{"IsNull", gst.FilterIsNull("expired_at"), gst.NewFilter("", "expired_at", gst.FilterOpIsNull, true)},
		{"IsNotNull", gst.FilterIsNotNull("expired_at"), gst.NewFilter("", "expired_at", gst.FilterOpIsNull, false)},
		{"Regex", gst.FilterRegex("name", "^sam"), gst.NewFilter("", "name", gst.FilterOpRegex, "^sam")},
		{"NotRegex", gst.FilterNotRegex("name", "^sam"), gst.NewFilter("", "name", gst.FilterOpNotRegex, "^sam")},
		{"JSONContains", gst.FilterJSONContains("tags", "sample"), gst.NewFilter("", "tags", gst.FilterOpJSONContains, "sample")},
		{"False", gst.FilterFalse(), gst.NewFilter("", "", gst.FilterOpFalse, nil)},
		{
			"Or",
			gst.FilterOr(gst.FilterEq("age", 18), gst.FilterEq("name", "sample")),
			gst.NewFilter("", "", gst.FilterOpOr, []gst.Filter{
				gst.NewFilter("", "age", gst.FilterOpEq, 18),
				gst.NewFilter("", "name", gst.FilterOpEq, "sample"),
			}),
		},
		{
			"And",
			gst.FilterAnd(gst.FilterEq("age", 18), gst.FilterEq("name", "sample")),
			gst.NewFilter("", "", gst.FilterOpAnd, []gst.Filter{
				gst.NewFilter("", "age", gst.FilterOpEq, 18),
				gst.NewFilter("", "name", gst.FilterOpEq, "sample"),
			}),
		},
		{
			"NestedGroups",
			gst.FilterOr(gst.FilterAnd(gst.FilterEq("age", 18))),
			gst.NewFilter("", "", gst.FilterOpOr, []gst.Filter{
				gst.NewFilter("", "", gst.FilterOpAnd, []gst.Filter{
					gst.NewFilter("", "age", gst.FilterOpEq, 18),
				}),
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestFilterGroupsKeepTheirOwnFilters(t *testing.T) {
	// Two groups built from one prefix slice with spare capacity keep their
	// own members: the second append does not rewrite the first.
	base := make([]gst.Filter, 0, 4)
	base = append(base, gst.FilterEq("status", "done"))
	or := gst.FilterOr(append(base, gst.FilterEq("kind", "gold"))...)
	and := gst.FilterAnd(append(base, gst.FilterEq("kind", "silver"))...)
	exists := gst.FilterExists[*sampleRecord](append(base, gst.FilterEq("kind", "bronze"))...)
	orMembers, ok := or.Value().([]gst.Filter)
	require.True(t, ok)
	andMembers, ok := and.Value().([]gst.Filter)
	require.True(t, ok)
	sub, ok := exists.Value().(gst.Subquery)
	require.True(t, ok)
	require.Equal(t, "gold", orMembers[1].Value())
	require.Equal(t, "silver", andMembers[1].Value())
	require.Equal(t, "bronze", sub.Filters[1].Value())
}

func TestFilterListsKeepTheirOwnValues(t *testing.T) {
	// Two lists built from one prefix slice with spare capacity keep their
	// own members: the second append does not rewrite the first.
	base := make([]string, 0, 4)
	base = append(base, "done")
	in := gst.FilterIn("status", append(base, "gold"))
	notIn := gst.FilterNotIn("status", append(base, "silver"))
	require.Equal(t, []string{"done", "gold"}, in.Value())
	require.Equal(t, []string{"done", "silver"}, notIn.Value())
}
