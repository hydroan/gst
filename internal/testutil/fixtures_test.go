package testutil_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// SampleRecord is the neutral database model the assertion helpers are tested
// against. It keeps the base default of soft deletion so the soft-delete
// assertions have a kept row to find.
type SampleRecord struct {
	Name string
	Tag  string

	modelregistry.Base
}

func (r *SampleRecord) TableName() string { return "testutil_sample_records" }

// createSampleRecord seeds one sample row and returns it. Callers keep names
// unique to their test so the shared table stays free of cross-test matches,
// and the row is purged once the test ends, so a repeated run (go test -count)
// finds the table as the first run did.
func createSampleRecord(t *testing.T, name, tag string) *SampleRecord {
	t.Helper()

	record := &SampleRecord{Name: name, Tag: tag}
	record.SetID()
	require.NoError(t, database.Database[*SampleRecord](t.Context()).Create(record))
	t.Cleanup(func() {
		// The test context is done by the time cleanups run.
		ctx := context.WithoutCancel(t.Context())
		require.NoError(t, database.Database[*SampleRecord](ctx).WithPurge(true).Delete(record))
	})
	return record
}
