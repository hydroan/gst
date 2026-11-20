package controller_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestListCallListsLikeTheHTTPQuery pins the list call: the query of the
// message is read the way the HTTP query is, a filter with an operator as
// field[op]=value, one without as the bare key, the orderings and the
// paging under their parameters; what the HTTP listener refuses, a filter on
// a field the model lacks, a page on a model that does not page, a filter
// given twice, is refused the same; and a hook's refusal answers with the
// hook's status.
func TestListCallListsLikeTheHTTPQuery(t *testing.T) {
	conn := sampleServer(t)
	prefix := uniqueName("call-list")
	first := createSample(t, prefix+"-a")
	second := createSample(t, prefix+"-b")

	listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
		"Filters": []map[string]any{{"Field": "name", "Op": "like", "Values": []string{prefix}}},
		"SortBy":  []string{"name desc"},
	}})
	require.NoError(t, err)
	require.EqualValues(t, 2, listed["total"])
	require.Equal(t, []string{second.GetID(), first.GetID()}, ids(listed["items"]))

	t.Run("a filter without an operator is the bare key", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Values": []string{prefix + "-a"}}},
		}})
		require.NoError(t, err)
		require.Equal(t, []string{first.GetID()}, ids(listed["items"]))
	})

	t.Run("a page of a model that pages", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "startswith", "Values": []string{prefix}}},
			"SortBy":  []string{"name"},
			"Page":    2,
			"Size":    1,
		}})
		require.NoError(t, err)
		require.EqualValues(t, 2, listed["total"])
		require.Equal(t, []string{second.GetID()}, ids(listed["items"]))
	})

	for _, tt := range []struct {
		name  string
		rpc   string
		query map[string]any
	}{
		{name: "a filter on a field the model lacks", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "missing", "Op": "eq", "Values": []string{"value"}}},
		}},
		{name: "a page on a model that does not page", rpc: "CounterList", query: map[string]any{"Page": 2}},
		{name: "a filter given twice", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a"}}, {"Field": "name", "Op": "eq", "Values": []string{"b"}}},
		}},
		{name: "several values under an operator taking one", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a", "b"}}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := invoke(t, conn, tt.rpc, map[string]any{"query": tt.query})
			requireStatus(t, err, codes.InvalidArgument, "")
		})
	}

	t.Run("a hook refusal answers with the hook's status", func(t *testing.T) {
		for _, rpc := range []string{"RefusedList", "FilterRefusedList"} {
			_, err := invoke(t, conn, rpc, map[string]any{})
			requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		}
	})
}
