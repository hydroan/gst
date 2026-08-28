package dcache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/config"
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

// TestCacheRequiresKafkaEnabled asserts the exported constructor fails fast
// when kafka is disabled, instead of handing out an instance that silently
// degrades to a process-local cache.
func TestCacheRequiresKafkaEnabled(t *testing.T) {
	old := config.App.Kafka.Enabled
	config.App.Kafka.Enabled = false
	defer func() { config.App.Kafka.Enabled = old }()

	_, err := Cache[struct{ Disabled int }]()
	require.Error(t, err, "a disabled kafka must fail the construction")
}

// TestWriteRejectsInvalidUTF8Key pins the key contract: a key that JSON
// would silently rewrite (invalid UTF-8 becomes U+FFFD) must be rejected up
// front, or the peers would apply the operation to a different key than the
// local store did.
func TestWriteRejectsInvalidUTF8Key(t *testing.T) {
	ctx := context.Background()

	dc, err := newReplicatedCache[int](newFakeStore[int]())
	require.NoError(t, err)

	badKey := "a\xff\xfeb"
	require.Error(t, dc.Set(ctx, badKey, 1, time.Minute))
	require.Error(t, dc.Delete(ctx, badKey))
	require.False(t, dc.Exists(ctx, badKey), "the rejected write must not reach the store")
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

	// A peer event older than the local write must be rejected as stale, and
	// the newer local value must survive it.
	stale, err := dc.applyPeerSet(&event{TS: time.Now().Add(-time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}, 99)
	require.NoError(t, err)
	require.True(t, stale, "a peer event older than the local write must be rejected")
	got, err := dc.Get(ctx, "watermark-key")
	require.NoError(t, err)
	require.Equal(t, 1, got, "the newer local write must survive a stale peer event")

	// A peer event newer than the local write lands.
	stale, err = dc.applyPeerSet(&event{TS: time.Now().Add(time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}, 2)
	require.NoError(t, err)
	require.False(t, stale, "a peer event newer than the local write must pass")

	// After a local delete, a stale peer set must not resurrect the key.
	require.NoError(t, dc.Delete(ctx, "watermark-key"))
	stale, err = dc.applyPeerSet(&event{TS: time.Now().Add(-time.Second).UnixNano(), Op: opSet, Key: "watermark-key", Typ: dc.typ, CacheID: "peer"}, 3)
	require.NoError(t, err)
	require.True(t, stale, "a stale peer set must not resurrect a locally deleted key")
	require.False(t, dc.Exists(ctx, "watermark-key"))
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

// TestWatermarkAdvancesOnlyOnApply asserts that a failed store application
// does not claim the event's timestamp: the same event still applies on a
// redelivery instead of being rejected as stale.
func TestWatermarkAdvancesOnlyOnApply(t *testing.T) {
	store := newFakeStore[uint]()
	dc, err := newReplicatedCache[uint](store)
	require.NoError(t, err)

	evt := &event{TS: 100, Op: opSet, Key: "apply-key", Typ: dc.typ, CacheID: "peer"}

	store.failSets = 1
	stale, err := dc.applyPeerSet(evt, 7)
	require.Error(t, err, "the injected store failure must surface")
	require.False(t, stale)

	// The watermark is untouched: the same timestamp still applies.
	stale, err = dc.applyPeerSet(evt, 7)
	require.NoError(t, err)
	require.False(t, stale)

	// Now the timestamp is claimed and a redelivery is rejected.
	stale, err = dc.applyPeerSet(evt, 7)
	require.NoError(t, err)
	require.True(t, stale)
}

// fakeStore is a minimal map-backed store giving the peer a private key
// space in TestPeerPropagation. Entries never expire here; the ttl of every
// write is recorded instead, so the test can assert what the event carried.
type fakeStore[T any] struct {
	mu   sync.Mutex
	m    map[string]entry[T]
	ttls map[string]time.Duration
	// failSets makes that many upcoming Set calls fail, for the tests that
	// assert a failed application claims nothing.
	failSets int
}

func newFakeStore[T any]() *fakeStore[T] {
	return &fakeStore[T]{m: make(map[string]entry[T]), ttls: make(map[string]time.Duration)}
}

func (s *fakeStore[T]) Set(_ context.Context, key string, value entry[T], ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSets > 0 {
		s.failSets--
		return errors.New("injected store failure")
	}
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
