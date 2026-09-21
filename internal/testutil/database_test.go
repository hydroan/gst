package testutil_test

import (
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestRequireGetReturnsTheRow(t *testing.T) {
	created := createSampleRecord(t, "get-target", "get")

	got := testutil.RequireGet[SampleRecord, *SampleRecord](t, created.ID)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "get-target", got.Name)
}

func TestRequireFirstReturnsTheMatch(t *testing.T) {
	createSampleRecord(t, "first-other", "first")
	created := createSampleRecord(t, "first-target", "first")

	got := testutil.RequireFirst(t, &SampleRecord{Name: "first-target"})
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "first", got.Tag)
}

func TestRequireListReturnsMatchesInOrder(t *testing.T) {
	createSampleRecord(t, "list-b", "list")
	createSampleRecord(t, "list-a", "list")
	createSampleRecord(t, "list-c", "list")

	rows := testutil.RequireList(t, &SampleRecord{Tag: "list"}, types.Asc("name"))
	require.Len(t, rows, 3)
	require.Equal(t, "list-a", rows[0].Name)
	require.Equal(t, "list-b", rows[1].Name)
	require.Equal(t, "list-c", rows[2].Name)
}

func TestRequireCountCountsTheMatches(t *testing.T) {
	createSampleRecord(t, "count-a", "count")
	createSampleRecord(t, "count-b", "count")

	require.Equal(t, 2, testutil.RequireCount(t, &SampleRecord{Tag: "count"}))
	require.Zero(t, testutil.RequireCount(t, &SampleRecord{Tag: "count-none"}))
}

func TestRequireListWithZeroValueQueryMatchesNothing(t *testing.T) {
	createSampleRecord(t, "zero-query-row", "zero")

	// WithQuery keeps the framework's empty-query safety check: a zero-value
	// query turns into "1 = 0" instead of matching the whole table.
	require.Empty(t, testutil.RequireList(t, &SampleRecord{}))
}

func TestRequireNoRowSeesHardDeletion(t *testing.T) {
	created := createSampleRecord(t, "hard-delete-target", "norow")

	require.NoError(t, database.Database[*SampleRecord](t.Context()).WithPurge(true).Delete(created))
	testutil.RequireNoRow[SampleRecord, *SampleRecord](t, created.ID)
}

func TestRequireNoRowSeesSoftDeletion(t *testing.T) {
	created := createSampleRecord(t, "soft-hidden-target", "norow")

	require.NoError(t, database.Database[*SampleRecord](t.Context()).Delete(created))
	testutil.RequireNoRow[SampleRecord, *SampleRecord](t, created.ID)
}

func TestRequireSoftDeletedSeesTheKeptRow(t *testing.T) {
	created := createSampleRecord(t, "soft-delete-target", "softdel")

	require.NoError(t, database.Database[*SampleRecord](t.Context()).Delete(created))
	testutil.RequireSoftDeleted[SampleRecord, *SampleRecord](t, created.ID)
}
