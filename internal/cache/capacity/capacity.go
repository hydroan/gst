// Package capacity resolves the per-type entry bound shared by the
// entry-addressed cache backends, so the configuration is read and validated
// in one place rather than copied into each of them.
package capacity

import "github.com/hydroan/gst/config"

const (
	// Default bounds every per-type cache when the configuration says
	// nothing. Instances are created lazily per value type, so only the types
	// a process actually caches pay for one.
	Default = 100_000

	// Max caps what the configuration can ask for. Backends size internal
	// tables from this number — one allocates ten admission counters per
	// entry — so an unbounded value turns a configuration typo into an
	// out-of-memory crash at startup, and a large enough one overflows the
	// 32-bit capacity some backends take.
	Max = 10_000_000
)

// Entries returns the configured per-type entry bound, clamped to a usable
// range. A non-positive setting means "unset" and yields Default.
func Entries() int {
	v := config.App.Cache.MaxEntries
	if v <= 0 {
		return Default
	}
	if v > Max {
		return Max
	}
	return v
}

// Entries32 returns Entries for backends whose capacity argument is 32-bit.
// The clamp in Entries is what makes the conversion safe.
func Entries32() uint32 {
	return uint32(Entries()) //nolint:gosec // Entries is clamped to Max, far below the 32-bit ceiling
}
