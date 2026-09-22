package entry_test

import (
	"testing"

	"demo/model/tool"
	"demo/service/tool/entry"

	"github.com/stretchr/testify/require"
)

// TestMergeKeepsTheLastValueOfAKey checks the merge rule of Merge.Create: a
// later value for a key replaces the earlier one, and every key keeps the
// position it first appeared at.
func TestMergeKeepsTheLastValueOfAKey(t *testing.T) {
	rsp, err := new(entry.Merge).Create(nil, &tool.EntryMergeReq{Entries: []tool.EntryPair{
		{Key: "a", Value: "1"},
		{Key: "b", Value: "2"},
		{Key: "a", Value: "3"},
	}})
	require.NoError(t, err)
	require.Equal(t, []tool.EntryPair{
		{Key: "a", Value: "3"},
		{Key: "b", Value: "2"},
	}, rsp.Entries)
}
