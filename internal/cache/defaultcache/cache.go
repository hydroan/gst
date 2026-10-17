// Package defaultcache is the in-memory cache backend the framework forwards
// as its default, freelru: the one place the choice is made. The public cache
// package forwards it to projects, with the reasons for the choice, and the
// framework's own code reads it from here.
package defaultcache

import (
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/internal/types"
)

// Cache returns the process-wide cache of T, the one instance the process
// keeps for the type, backed by the default backend.
func Cache[T any]() types.Cache[T] {
	return freelru.Cache[T]()
}
