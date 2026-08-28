package dcache

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// typedProbe is the struct proving typed values survive the kafka round trip.
type typedProbe struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

// TestPeerPropagation is the regression guard for the kafka broadcast: a set
// or delete on one instance reaches the local tier of a peer instance of the
// same type, with the value decoded into T.
func TestPeerPropagation(t *testing.T) {
	ctx := context.Background()

	// The peer is built through the internal constructor: the exported one
	// keeps a single instance per type. It also gets a private local tier,
	// because NewLocalCache is one store per type and sharing it with the
	// source would satisfy the assertions without any kafka round trip.
	source, err := newDistributedCache[typedProbe]()
	require.NoError(t, err)
	peerLocal, err := ristretto.NewCache(buildConf[typedProbe]())
	require.NoError(t, err)
	peer, err := newDistributedCache(WithLocalCache[typedProbe](&localCache[typedProbe]{c: peerLocal}))
	require.NoError(t, err)

	want := typedProbe{Name: "typed", Num: 42}
	var got typedProbe
	require.Eventually(t, func() bool {
		// Republish on every probe: consumers start at the end of the topic,
		// so an event published before the peer's group has joined would
		// never be delivered to it.
		if err := source.Set(ctx, "propagated-key", want, time.Minute); err != nil {
			return false
		}
		val, err := peer.Get(ctx, "propagated-key")
		if err != nil {
			return false
		}
		got = val
		return true
	}, 30*time.Second, 100*time.Millisecond, "the set event must reach the peer's local tier")
	require.Equal(t, want, got)

	// A delete on the source removes the entry from the peer as well. The
	// republished deletes carry ever newer timestamps, so any set event still
	// in flight from the loop above is rejected as stale rather than
	// resurrecting the entry.
	require.Eventually(t, func() bool {
		if err := source.Delete(ctx, "propagated-key"); err != nil {
			return false
		}
		_, err := peer.Get(ctx, "propagated-key")
		return errors.Is(err, types.ErrEntryNotFound)
	}, 30*time.Second, 100*time.Millisecond, "the delete event must reach the peer's local tier")
}
