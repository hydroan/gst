package tool_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/tool"

	"github.com/stretchr/testify/require"
)

// TestMerge covers POST /api/entries/merge, served by Merge in merge.go: a
// later value for a key wins, and the keys keep their first positions.
func TestMerge(t *testing.T) {
	account := testsupport.Login(t)

	rsp, err := account.Client.Post[tool.EntryMergeRsp](t.Context(), "/api/entries/merge", &tool.EntryMergeReq{Entries: []tool.EntryPair{
		{Key: "color", Value: "red"},
		{Key: "size", Value: "small"},
		{Key: "color", Value: "blue"},
	}})
	require.NoError(t, err)
	require.Equal(t, []tool.EntryPair{{Key: "color", Value: "blue"}, {Key: "size", Value: "small"}}, rsp.Entries)
}
