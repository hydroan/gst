package cache

import (
	"github.com/hydroan/gst/cache/bigcache"
	"github.com/hydroan/gst/cache/ccache"
	"github.com/hydroan/gst/cache/cmap"
	"github.com/hydroan/gst/cache/fastcache"
	"github.com/hydroan/gst/cache/freecache"
	"github.com/hydroan/gst/cache/gocache"
	"github.com/hydroan/gst/cache/lru"
	"github.com/hydroan/gst/cache/lrue"
	"github.com/hydroan/gst/cache/ristretto"
	"github.com/hydroan/gst/cache/smap"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

// Init initialize all cache implementations.
//
// # Cache Implementations Overview
//
// | Package     | Expiration Strategy       |
// |-------------|---------------------------|
// | lru         | No expiration             |
// | cmap        | No expiration             |
// | smap        | No expiration             |
// | fastcache   | No expiration             |
// | lrue        | Global expiration         |
// | bigcache    | Global expiration         |
// | ristretto   | Per-entry expiration      |
// | freecache   | Per-entry expiration      |
// | ccache      | Per-entry expiration      |
// | gocache     | Per-entry expiration      |
func Init() error {
	return util.CombineError(
		// ---- No expiration (eviction only by capacity or usage) ----
		lru.Init,
		cmap.Init,
		smap.Init,
		fastcache.Init,

		// ---- Global expiration (single TTL for all entries) ----
		lrue.Init,
		bigcache.Init,

		// ---- Per-entry expiration (each entry can have its own TTL) ----
		ristretto.Init,
		ccache.Init,
		gocache.Init,
		freecache.Init,
	)
}

func Cache[T any]() types.Cache[T]          { return lrue.Cache[T]() }
func ExpirableCache[T any]() types.Cache[T] { return ristretto.Cache[T]() }
