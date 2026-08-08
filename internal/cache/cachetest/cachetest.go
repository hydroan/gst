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
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
)

// FillTestConfig fills the cache configuration the backends read at
// construction, so backend test binaries can run without the full config
// bootstrap. Call it from TestMain before any backend constructor runs.
func FillTestConfig() {
	config.App.Cache.Capacity = 100000
	config.App.Cache.Expiration = 10 * time.Minute
	config.App.Cache.CleanWindow = 5 * time.Minute
	config.App.Cache.LifeWindow = 10 * time.Minute
	config.App.Cache.Shards = 16
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
