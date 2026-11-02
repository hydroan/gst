package record_test

import (
	"testing"

	"stress/internal/testsupport"
	"stress/model"

	"github.com/stretchr/testify/require"
)

// TestUpsertInsertsThenOverwritesTheRecordOfTheCode covers POST
// /api/records/upsert, served by Upsert in upsert.go: the first call inserts
// the record under the id its code names, the second overwrites that row
// instead of adding one.
func TestUpsertInsertsThenOverwritesTheRecordOfTheCode(t *testing.T) {
	cli := testsupport.Login(t).Client

	first, err := cli.Post[model.Record](t.Context(), "/api/records/upsert", &model.RecordUpsertReq{Code: "u-1", Title: "first", Category: "alpha", Score: 1})
	require.NoError(t, err)
	require.Equal(t, "rec-u-1", first.ID)

	second, err := cli.Post[model.Record](t.Context(), "/api/records/upsert", &model.RecordUpsertReq{Code: "u-1", Title: "second", Category: "alpha", Score: 2})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	stored, err := cli.Get[model.Record](t.Context(), "/api/records/rec-u-1")
	require.NoError(t, err)
	require.Equal(t, "second", stored.Title)
	require.Equal(t, 2, stored.Score)
}
