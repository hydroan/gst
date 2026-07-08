package serviceregistry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestQueryDecoderIsShared guards the caching contract: QueryDecoder must
// return the same instance on every call so gorilla/schema can reuse its
// struct-metadata cache across requests instead of rebuilding it per call.
func TestQueryDecoderIsShared(t *testing.T) {
	first := QueryDecoder()
	second := QueryDecoder()

	require.Same(t, first, second)
}

// TestQueryDecoderUsesQueryTag verifies the shared decoder maps fields by the
// "query" alias tag rather than gorilla/schema's default "schema" tag.
func TestQueryDecoderUsesQueryTag(t *testing.T) {
	type params struct {
		Name string `query:"name"`
	}

	var got params
	err := QueryDecoder().Decode(&got, map[string][]string{"name": {"alice"}})

	require.NoError(t, err)
	require.Equal(t, "alice", got.Name)
}
