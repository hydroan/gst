package dcache

const (
	// maxTrackedKeys bounds each instance's per-key timestamp table; see the
	// package documentation for the eviction trade-off. It deliberately sits
	// an order of magnitude above the store's default entry bound: the
	// watermark must keep covering keys the store has already evicted or
	// expired, or a stale peer event could resurrect them.
	maxTrackedKeys = 1_000_000

	// minGoroutines is the floor of the event-publishing pool capacity.
	minGoroutines = 10000
)
