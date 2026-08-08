// Package registry provides the shared per-type singleton store used by the
// in-memory cache backends. Every backend package keeps one cache instance per
// value type; this package holds the lookup and double-checked creation logic
// so the backends do not each carry their own copy.
package registry

import (
	"reflect"
	"sync"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// Store keeps one instance per type key. The zero value is not usable; create
// stores with New.
type Store struct {
	m  cmap.ConcurrentMap[string, any]
	mu sync.Mutex
}

// New returns an empty Store.
func New() *Store {
	return &Store{m: cmap.New[any]()}
}

// Load returns the instance registered under type C, creating it with create
// on first use. Create runs at most once per type.
func Load[C any](s *Store, create func() C) C {
	key := typeKey[C]()
	if v, ok := s.m.Get(key); ok {
		return v.(C) //nolint:errcheck
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.m.Get(key); ok {
		return v.(C) //nolint:errcheck
	}
	v := create()
	s.m.Set(key, v)
	return v
}

func typeKey[C any]() string {
	typ := reflect.TypeFor[C]()
	return typ.PkgPath() + "|" + typ.String()
}
