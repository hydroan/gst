// Package capacity resolves the per-type entry bound shared by the
// entry-addressed cache backends, so the configuration is read in one place
// rather than copied into each of them.
package capacity

import "github.com/hydroan/gst/config"

// Default bounds every per-type cache when the configuration says nothing.
// Instances are created lazily per value type, so only the types a process
// actually caches pay for one.
const Default = 100_000

// Entries returns the configured per-type entry bound. A non-positive
// setting means "unset" and yields Default; the configuration holds the
// setting to at most ten million, which fits the 32-bit capacity some
// backends take.
func Entries() int {
	if v := config.App.Cache.MaxEntries; v > 0 {
		return v
	}
	return Default
}

// Entries32 returns Entries for backends whose capacity argument is 32-bit.
func Entries32() uint32 {
	return uint32(Entries()) //nolint:gosec // the configuration holds the setting under ten million, far below the 32-bit ceiling
}
