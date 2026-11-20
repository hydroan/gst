package controller_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestUpdateCallReplacesTheRecord pins the update call: the record the id
// names is replaced by the message's, its creation audit kept; an id no
// record carries answers NotFound; and a message carrying no record is
// refused, the record left as stored rather than replaced by a zero one.
func TestUpdateCallReplacesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-update")

	updated, err := invoke(t, conn, "Update", map[string]any{"id": record.GetID(), "record": map[string]any{"name": "call-updated"}})
	require.NoError(t, err)
	require.Equal(t, "call-updated", updated["name"])
	require.Equal(t, "u-1", updated["updated_by"])
	requireSampleName(t, record.GetID(), "call-updated")

	_, err = invoke(t, conn, "Update", map[string]any{"id": "missing", "record": map[string]any{"name": "call-updated"}})
	requireStatus(t, err, codes.NotFound, "")

	_, err = invoke(t, conn, "Update", map[string]any{"id": record.GetID()})
	requireStatus(t, err, codes.InvalidArgument, "record is required")
	requireSampleName(t, record.GetID(), "call-updated")

	t.Run("a record naming another id is written under the id of the call", func(t *testing.T) {
		other := createSample(t, "call-update-other")

		updated, err := invoke(t, conn, "Update", map[string]any{"id": record.GetID(), "record": map[string]any{"id": other.GetID(), "name": "call-moved"}})
		require.NoError(t, err)
		require.Equal(t, record.GetID(), updated["id"])
		requireSampleName(t, record.GetID(), "call-moved")
		requireSampleName(t, other.GetID(), "call-update-other")
	})
}
