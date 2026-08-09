// Package cachetest provides the conformance suite every types.Cache backend
// must pass. Each backend declares its capabilities and the suite asserts the
// contract documented on types.Cache: miss errors, idempotent deletes, ttl
// semantics and concurrency safety.
package cachetest

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

const (
	// warmPasses is how many times each resident entry is read before the
	// newcomer probes run, so frequency-based admission has accumulated hits
	// to weigh newcomers against.
	warmPasses = 20

	// visibilityProbes is how many newcomer writes the visibility check makes.
	visibilityProbes = 200
)

// RunWriteVisibility asserts the contract's core write guarantee under
// capacity pressure: a value stored by Set must be readable by the next Get,
// even when the cache is full and its resident entries are warm.
//
// It is separate from Run because it needs an instance whose capacity it
// knows and whose contents it owns, so callers pass a freshly created cache —
// giving the probe its own value type yields its own per-type instance —
// together with the entry bound that instance was built with.
//
// The warm-up pass is what makes this a real guard rather than a formality.
// A backend that admits entries by estimated frequency only starts rejecting
// newcomers once the resident entries have accumulated hits; merely filling
// the cache to capacity leaves the defect invisible, which is why the rest of
// this suite passes even on a backend that drops most of its writes.
func RunWriteVisibility[T ~string](t *testing.T, cache types.Cache[T], capacity int) {
	t.Helper()
	ctx := context.Background()

	for i := range capacity {
		if err := cache.Set(ctx, "resident-"+strconv.Itoa(i), T("resident"), 0); err != nil {
			t.Fatalf("filling the cache: %v", err)
		}
	}
	for range warmPasses {
		for i := range capacity {
			_, _ = cache.Get(ctx, "resident-"+strconv.Itoa(i))
		}
	}

	var lost int
	for i := range visibilityProbes {
		key := "newcomer-" + strconv.Itoa(i)
		if err := cache.Set(ctx, key, T("newcomer"), 0); err != nil {
			t.Fatalf("probe set: %v", err)
		}
		if _, err := cache.Get(ctx, key); err != nil {
			lost++
		}
	}
	if lost != 0 {
		t.Fatalf("%d of %d writes reported success but are not readable", lost, visibilityProbes)
	}
}

// RunWriteRetention asserts that entries written under capacity pressure are
// still there when a later request comes looking, not merely readable on the
// next line.
//
// RunWriteVisibility cannot see this. Reading each key immediately after
// writing it hides any backend that accepts the write and then picks the
// newcomer as its eviction victim, because the read lands before the eviction
// does. An admission-weighted policy does exactly that: a fresh key has the
// lowest frequency estimate in the cache, so it is the first thing evicted
// once the writes stop. Measured on a warm 100k-entry cache taking 10k new
// keys, one backend lost none on the immediate read and 44% on a read 200ms
// later.
//
// Preferring old entries over new ones is a legitimate policy — it is the
// same mechanism that makes a cache resist scans — but it is incompatible
// with the ordinary write-now-read-later use of a cache, so the forwarded
// default has to pass this.
func RunWriteRetention[T ~string](t *testing.T, cache types.Cache[T], capacity int) {
	t.Helper()
	ctx := context.Background()

	for i := range capacity {
		if err := cache.Set(ctx, "resident-"+strconv.Itoa(i), T("resident"), 0); err != nil {
			t.Fatalf("filling the cache: %v", err)
		}
	}
	for range warmPasses {
		for i := range capacity {
			_, _ = cache.Get(ctx, "resident-"+strconv.Itoa(i))
		}
	}

	for i := range visibilityProbes {
		if err := cache.Set(ctx, "retained-"+strconv.Itoa(i), T("retained"), 0); err != nil {
			t.Fatalf("probe set: %v", err)
		}
	}
	time.Sleep(200 * time.Millisecond)

	var lost int
	for i := range visibilityProbes {
		if _, err := cache.Get(ctx, "retained-"+strconv.Itoa(i)); err != nil {
			lost++
		}
	}
	if lost != 0 {
		t.Fatalf("%d of %d entries were evicted before a later read could reach them", lost, visibilityProbes)
	}
}

// Capabilities declares which parts of the ttl contract a backend honors.
type Capabilities struct {
	// PerEntryTTL is true when Set honors ttl > 0 as a per-entry lifetime.
	PerEntryTTL bool

	// NoExpiry is true when Set honors ttl == 0 as "never expires".
	NoExpiry bool

	// TTLGranularity is the coarsest ttl unit the backend can represent; zero
	// means full precision. Second-granularity backends round sub-second ttls
	// up, so the expiry probe waits accordingly.
	TTLGranularity time.Duration

	// MaxEntries is the backend's entry bound when it has one, and the suite
	// overfills it to prove eviction runs. Zero means the backend is not
	// bounded by entry count — either it evicts by byte budget, which takes
	// too long to fill in a conformance run, or it never evicts at all.
	// Unbounded separates those two, so a backend that quietly stopped
	// enforcing its bound cannot pass as one that never claimed one.
	MaxEntries int

	// Unbounded declares that the backend never evicts, so a caller-influenced
	// key space grows until the process runs out of memory.
	Unbounded bool
}

// Run asserts the types.Cache contract against cache. Backends that honor
// neither ttl form must reject every Set with ErrTTLNotSupported and are only
// checked for that plus the read-path contract.
func Run(t *testing.T, cache types.Cache[string], caps Capabilities) {
	t.Helper()
	ctx := context.Background()

	t.Run("GetMissingReturnsErrEntryNotFound", func(t *testing.T) {
		if _, err := cache.Get(ctx, "conformance-missing"); !errors.Is(err, types.ErrEntryNotFound) {
			t.Fatalf("want ErrEntryNotFound, got %v", err)
		}
	})

	t.Run("DeleteMissingIsIdempotent", func(t *testing.T) {
		if err := cache.Delete(ctx, "conformance-missing"); err != nil {
			t.Fatalf("delete missing key: %v", err)
		}
	})

	t.Run("NilContextIsTreatedAsBackground", func(t *testing.T) {
		// The contract promises a nil ctx behaves as context.Background(); a
		// typed nil variable dodges the linter that rejects literal nils.
		var nilCtx context.Context
		if _, err := cache.Get(nilCtx, "conformance-nil-ctx"); !errors.Is(err, types.ErrEntryNotFound) {
			t.Fatalf("want ErrEntryNotFound, got %v", err)
		}
		if cache.Exists(nilCtx, "conformance-nil-ctx") {
			t.Fatal("want false for missing key")
		}
		if err := cache.Delete(nilCtx, "conformance-nil-ctx"); err != nil {
			t.Fatalf("delete with nil ctx: %v", err)
		}
	})

	t.Run("ExistsMissingIsFalse", func(t *testing.T) {
		if cache.Exists(ctx, "conformance-missing") {
			t.Fatal("want false for missing key")
		}
	})

	if !caps.PerEntryTTL && !caps.NoExpiry {
		t.Run("SetAlwaysReturnsErrTTLNotSupported", func(t *testing.T) {
			if err := cache.Set(ctx, "conformance-unsupported", "value", 0); !errors.Is(err, types.ErrTTLNotSupported) {
				t.Fatalf("want ErrTTLNotSupported for ttl=0, got %v", err)
			}
			if err := cache.Set(ctx, "conformance-unsupported", "value", time.Minute); !errors.Is(err, types.ErrTTLNotSupported) {
				t.Fatalf("want ErrTTLNotSupported for ttl>0, got %v", err)
			}
		})
		return
	}

	storeTTL := time.Duration(0)
	if !caps.NoExpiry {
		storeTTL = time.Hour
	}

	t.Run("NegativeTTLIsRejected", func(t *testing.T) {
		if err := cache.Set(ctx, "conformance-negative", "value", -time.Second); err == nil {
			t.Fatal("want error for negative ttl, got nil")
		}
	})

	t.Run("SetGetRoundtrip", func(t *testing.T) {
		if err := cache.Set(ctx, "conformance-roundtrip", "value", storeTTL); err != nil {
			t.Fatalf("set: %v", err)
		}
		got, err := cache.Get(ctx, "conformance-roundtrip")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got != "value" {
			t.Fatalf("want %q, got %q", "value", got)
		}
	})

	t.Run("ExistsReflectsPresence", func(t *testing.T) {
		if err := cache.Set(ctx, "conformance-exists", "value", storeTTL); err != nil {
			t.Fatalf("set: %v", err)
		}
		if !cache.Exists(ctx, "conformance-exists") {
			t.Fatal("want true for stored key")
		}
	})

	t.Run("DeleteRemovesEntry", func(t *testing.T) {
		if err := cache.Set(ctx, "conformance-delete", "value", storeTTL); err != nil {
			t.Fatalf("set: %v", err)
		}
		if err := cache.Delete(ctx, "conformance-delete"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := cache.Get(ctx, "conformance-delete"); !errors.Is(err, types.ErrEntryNotFound) {
			t.Fatalf("want ErrEntryNotFound after delete, got %v", err)
		}
		if err := cache.Delete(ctx, "conformance-delete"); err != nil {
			t.Fatalf("second delete must stay idempotent: %v", err)
		}
	})

	if caps.PerEntryTTL {
		t.Run("PositiveTTLExpires", func(t *testing.T) {
			ttl := max(200*time.Millisecond, caps.TTLGranularity)
			if err := cache.Set(ctx, "conformance-expiry", "value", ttl); err != nil {
				t.Fatalf("set: %v", err)
			}
			if _, err := cache.Get(ctx, "conformance-expiry"); err != nil {
				t.Fatalf("get before expiry: %v", err)
			}
			time.Sleep(ttl + caps.TTLGranularity + 300*time.Millisecond)
			if _, err := cache.Get(ctx, "conformance-expiry"); !errors.Is(err, types.ErrEntryNotFound) {
				t.Fatalf("want ErrEntryNotFound after expiry, got %v", err)
			}
		})
	} else {
		t.Run("PositiveTTLReturnsErrTTLNotSupported", func(t *testing.T) {
			if err := cache.Set(ctx, "conformance-expiry", "value", time.Minute); !errors.Is(err, types.ErrTTLNotSupported) {
				t.Fatalf("want ErrTTLNotSupported, got %v", err)
			}
		})
	}

	if caps.NoExpiry {
		t.Run("ZeroTTLDoesNotExpireQuickly", func(t *testing.T) {
			if err := cache.Set(ctx, "conformance-forever", "value", 0); err != nil {
				t.Fatalf("set: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			if _, err := cache.Get(ctx, "conformance-forever"); err != nil {
				t.Fatalf("ttl=0 entry must survive: %v", err)
			}
		})
	} else {
		t.Run("ZeroTTLReturnsErrTTLNotSupported", func(t *testing.T) {
			if err := cache.Set(ctx, "conformance-forever", "value", 0); !errors.Is(err, types.ErrTTLNotSupported) {
				t.Fatalf("want ErrTTLNotSupported, got %v", err)
			}
		})
	}

	t.Run("CapacityEvicts", func(t *testing.T) {
		if caps.Unbounded {
			t.Skip("backend declares that it never evicts")
		}
		if caps.MaxEntries == 0 {
			t.Skip("backend is not bounded by entry count")
		}
		// Overfilling by a wide margin proves eviction runs without pinning
		// the backend to an exact resident count: sharded and asynchronous
		// eviction both overshoot a little by design.
		limit := caps.MaxEntries * 3
		for i := range limit {
			_ = cache.Set(ctx, "bound-"+strconv.Itoa(i), "value", storeTTL)
		}
		var resident int
		for i := range limit {
			if cache.Exists(ctx, "bound-"+strconv.Itoa(i)) {
				resident++
			}
		}
		if resident >= limit {
			t.Fatalf("want eviction after %d writes, all %d are still resident", limit, resident)
		}
	})

	// The concurrency probe is the regression guard for the data race the old
	// interface carried: every operation used to rewrite a shared ctx field.
	// Run the suite with -race to arm it. Operation errors are ignored on
	// purpose; admission-based backends may reject writes under contention.
	t.Run("ConcurrentAccess", func(t *testing.T) {
		var wg sync.WaitGroup
		for worker := range 8 {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := range 300 {
					key := "conformance-concurrent-" + strconv.Itoa(i%16)
					switch (worker + i) % 4 {
					case 0:
						_ = cache.Set(ctx, key, "value", storeTTL)
					case 1:
						_, _ = cache.Get(ctx, key)
					case 2:
						cache.Exists(ctx, key)
					default:
						_ = cache.Delete(ctx, key)
					}
				}
			}(worker)
		}
		wg.Wait()
	})
}
