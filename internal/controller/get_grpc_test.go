package controller_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestGetCallAnswersTheRecordOrNotFound pins the get call: the record the id
// names, NotFound for an id no record carries or the integer key cannot
// hold, and a refusal for a message naming no id at all.
func TestGetCallAnswersTheRecordOrNotFound(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-get")

	got, err := invoke(t, conn, "Get", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Equal(t, record.GetID(), got["id"])
	require.Equal(t, "call-get", got["name"])

	t.Run("an id no record carries", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{"id": "missing"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		_, err := invoke(t, conn, "CounterGet", map[string]any{"id": "first"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("no id at all", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{})
		requireStatus(t, err, codes.InvalidArgument, "id is required")
	})
}
