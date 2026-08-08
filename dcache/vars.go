package dcache

const (
	// maxTrackedKeys bounds the state node's per-key timestamp table; see the
	// package documentation for the eviction trade-off.
	maxTrackedKeys = 1_000_000

	// minGoroutines is the floor of the event-publishing pool capacity.
	minGoroutines = 10000
)
