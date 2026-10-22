package testutil_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/tenant"
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
	t.Run("the_base_soft_delete", func(t *testing.T) {
		created := createSampleRecord(t, "soft-delete-target", "softdel")

		require.NoError(t, database.Database[*SampleRecord](t.Context()).Delete(created))
		testutil.RequireSoftDeleted[SampleRecord, *SampleRecord](t, created.ID)
	})

	t.Run("a_zero_value_soft_delete_filed_under_another_tenant", func(t *testing.T) {
		created := createZeroValueRecord(t, "soft-delete-zero-value-target")

		require.NoError(t, database.Database[*ZeroValueRecord](tenant.In(t.Context(), otherTenant)).Delete(created))
		testutil.RequireSoftDeleted[ZeroValueRecord, *ZeroValueRecord](t, created.ID)
	})
}

// softDeletedHelper marks the child process that runs RequireSoftDeleted for
// TestRequireSoftDeletedRejectsARowThatIsNotSoftDeleted, naming the row it
// seeds.
const softDeletedHelper = "GST_TESTUTIL_SOFT_DELETED_HELPER"

// TestRequireSoftDeletedRejectsARowThatIsNotSoftDeleted pins that the
// assertion fails on a row no soft deletion touched: a live row filed under
// another tenant, whose soft-delete column holds the zero value rather than
// NULL, and a row deleted for good. The assertion fails the test it runs in,
// so it runs in a child process: the test binary runs itself again with this
// test selected and the row named.
func TestRequireSoftDeletedRejectsARowThatIsNotSoftDeleted(t *testing.T) {
	if row := os.Getenv(softDeletedHelper); row != "" {
		record := createZeroValueRecord(t, "rejected-"+row)
		if row == "hard_deleted_row" {
			require.NoError(t, database.Database[*ZeroValueRecord](tenant.In(t.Context(), otherTenant)).WithPurge(true).Delete(record))
		}
		testutil.RequireSoftDeleted[ZeroValueRecord, *ZeroValueRecord](t, record.ID)
		return
	}

	for row, failure := range map[string]string{
		"live_row_of_another_tenant": "the row should be gone from the regular query path of every tenant",
		"hard_deleted_row":           "the soft-deleted row should stay in",
	} {
		t.Run(row, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRequireSoftDeletedRejectsARowThatIsNotSoftDeleted$")
			cmd.Env = append(os.Environ(), softDeletedHelper+"="+row)
			out, err := cmd.CombinedOutput()

			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "RequireSoftDeleted must fail the test it runs in:\n%s", out)
			require.Contains(t, string(out), failure)
		})
	}
}
