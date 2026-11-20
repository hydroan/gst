package controller_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestDeleteCallDeletesTheRecord pins the delete call: the record the id
// names is gone afterwards, a hook's refusal keeps it, and a message naming
// no record is refused by the name of the field it lacks, id, not by the
// route parameter the handler carries the id under.
func TestDeleteCallDeletesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-delete")

	_, err := invoke(t, conn, "Delete", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, "call-delete"))

	_, err = invoke(t, conn, "Delete", map[string]any{})
	requireStatus(t, err, codes.InvalidArgument, "id is required")

	kept := createSample(t, "call-delete-refused")
	_, err = invoke(t, conn, "RefusedDelete", map[string]any{"id": kept.GetID()})
	requireStatus(t, err, codes.AlreadyExists, refusedMsg)
	requireSampleName(t, kept.GetID(), "call-delete-refused")
}
