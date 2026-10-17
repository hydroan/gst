package defaultcache_test

import (
	"testing"

	"github.com/hydroan/gst/internal/cache/defaultcache"
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/stretchr/testify/require"
)

// TestCacheIsTheFreelruCache pins the backend the framework forwards as its
// default, the choice the public cache package documents: the cache of a
// type is freelru's, the one instance the process keeps for the type.
func TestCacheIsTheFreelruCache(t *testing.T) {
	forwarded := defaultcache.Cache[int]()
	backend := freelru.Cache[int]()
	require.Same(t, backend, forwarded)
}
