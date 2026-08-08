package dcache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// typedProbe is the struct proving typed values survive the redis tier.
type typedProbe struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

// TestGetWithSyncRedisHitReturnsTypedValue is the regression guard for the
// redis tier: values decode into T directly, so a struct read from redis is
// returned instead of failing a type assertion and masking the entry.
func TestGetWithSyncRedisHitReturnsTypedValue(t *testing.T) {
	ctx := context.Background()

	handle, err := NewDistributedCache[typedProbe]()
	require.NoError(t, err)
	dc, ok := handle.(*distributedCache[typedProbe])
	require.True(t, ok)

	want := typedProbe{Name: "typed", Num: 42}
	// Plant the value in the redis tier directly: the kafka pipeline that
	// normally fills it is not part of this test environment.
	require.NoError(t, dc.redisCache.Set(ctx, dc.prefix+"redis-hit-key", want, time.Minute))

	got, err := dc.GetWithSync(ctx, "redis-hit-key", time.Minute)
	require.NoError(t, err)
	require.Equal(t, want, got)

	// The redis hit must be written back to the local tier.
	got, err = dc.Get(ctx, "redis-hit-key")
	require.NoError(t, err)
	require.Equal(t, want, got)
}
