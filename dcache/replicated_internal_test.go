package dcache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// typedProbe is the struct proving typed values survive the kafka round trip.
type typedProbe struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

// TestPeerPropagation is the regression guard for the kafka broadcast: a set
// or delete on one instance reaches the store of a peer instance of the same
// type, with the value decoded into T.
func TestPeerPropagation(t *testing.T) {
	ctx := context.Background()

	// The peer is built through the internal constructor: the exported one
	// keeps a single instance per type. It also gets a private store, because
	// the source's cache.Cache store is one instance per type and sharing it
	// would satisfy the assertions without any kafka round trip.
	source, err := newReplicatedCache[typedProbe](cache.Cache[entry[typedProbe]]())
	require.NoError(t, err)
	peerStore := newFakeStore[typedProbe]()
	peer, err := newReplicatedCache[typedProbe](peerStore)
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
	}, 30*time.Second, 100*time.Millisecond, "the set event must reach the peer's store")
	require.Equal(t, want, got)
	// The event carries the original ttl through to the peer store.
	require.Equal(t, time.Minute, peerStore.lastTTL("propagated-key"))

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
	}, 30*time.Second, 100*time.Millisecond, "the delete event must reach the peer's store")
}

// TestWatermarkCoversLocalWrites is the regression guard for the write-path
// watermark: local Set and Delete enter the same per-key watermark as peer
// events, so a stale peer event arriving later is rejected instead of
// overwriting the newer local write or resurrecting the deleted key.
func TestWatermarkCoversLocalWrites(t *testing.T) {
	ctx := context.Background()

	dc, err := newReplicatedCache[int](newFakeStore[int]())
	require.NoError(t, err)

	require.NoError(t, dc.Set(ctx, "watermark-key", 1, time.Minute))

	stale := &event{TS: time.Now().Add(-time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}
	require.False(t, dc.shouldApply(stale), "a peer event older than the local write must be rejected")

	fresh := &event{TS: time.Now().Add(time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}
	require.True(t, dc.shouldApply(fresh), "a peer event newer than the local write must pass")

	require.NoError(t, dc.Delete(ctx, "watermark-key"))
	staleSet := &event{TS: time.Now().Add(-time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}
	require.False(t, dc.shouldApply(staleSet), "a stale peer set must not resurrect a locally deleted key")
}

// TestSetReturnsMarshalFailure asserts that a value the event pipeline
// cannot serialize surfaces as an error instead of silently never reaching
// the peers; the local tier has already stored it by then.
func TestSetReturnsMarshalFailure(t *testing.T) {
	ctx := context.Background()

	dc, err := newReplicatedCache[chan int](newFakeStore[chan int]())
	require.NoError(t, err)

	err = dc.Set(ctx, "unmarshalable-key", make(chan int), time.Minute)
	require.Error(t, err, "a JSON-unmarshalable value must fail the publication")

	// The local tier holds the value regardless, as documented.
	require.True(t, dc.Exists(ctx, "unmarshalable-key"))
}

// TestWatermarkAdvancesOnlyOnApply asserts the judge/record split: passing
// shouldApply does not claim the timestamp, only advanceWatermark does, so a
// failed application leaves the watermark untouched.
func TestWatermarkAdvancesOnlyOnApply(t *testing.T) {
	dc, err := newReplicatedCache[uint](newFakeStore[uint]())
	require.NoError(t, err)

	evt := &event{TS: 100, Op: opSet, Key: "apply-key", Typ: dc.typ, CacheID: "peer"}
	require.True(t, dc.shouldApply(evt))
	// Not applied yet: the same timestamp must still pass.
	require.True(t, dc.shouldApply(evt))

	dc.advanceWatermark(evt.Key, evt.TS)
	require.False(t, dc.shouldApply(evt), "an applied timestamp must be rejected afterwards")
}

// fakeStore is a minimal map-backed store giving the peer a private key
// space in TestPeerPropagation. Entries never expire here; the ttl of every
// write is recorded instead, so the test can assert what the event carried.
type fakeStore[T any] struct {
	mu   sync.Mutex
	m    map[string]entry[T]
	ttls map[string]time.Duration
}

func newFakeStore[T any]() *fakeStore[T] {
	return &fakeStore[T]{m: make(map[string]entry[T]), ttls: make(map[string]time.Duration)}
}

func (s *fakeStore[T]) Set(_ context.Context, key string, value entry[T], ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	s.ttls[key] = ttl
	return nil
}

// lastTTL returns the ttl recorded by the latest Set of key.
func (s *fakeStore[T]) lastTTL(key string) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ttls[key]
}

func (s *fakeStore[T]) Get(_ context.Context, key string) (entry[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.m[key]
	if !ok {
		return entry[T]{}, types.ErrEntryNotFound
	}
	return value, nil
}

func (s *fakeStore[T]) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func (s *fakeStore[T]) Exists(_ context.Context, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	return ok
}
