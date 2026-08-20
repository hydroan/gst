package testutil

import (
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

// TestMain boots the framework against the default sqlite database and
// registers the sample model the database assertions run against.
func TestMain(m *testing.M) {
	Run(m, Server{
		Register: func() { modelregistry.EnqueueTable(&SampleRecord{}) },
	})
}

// createSampleRecord seeds one sample row and returns it. Callers keep names
// unique to their test so the shared table stays free of cross-test matches.
func createSampleRecord(t *testing.T, name, tag string) *SampleRecord {
	t.Helper()

	record := &SampleRecord{Name: name, Tag: tag}
	record.SetID()
	require.NoError(t, database.Database[*SampleRecord](t.Context()).Create(record))
	return record
}
