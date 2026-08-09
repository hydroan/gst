// Package registry provides the shared per-type singleton store used by the
// in-memory cache backends. Every backend package keeps one cache instance per
// value type; this package holds the lookup and double-checked creation logic
// so the backends do not each carry their own copy.
package registry

import (
	"reflect"
	"sync"
)

// Store keeps one instance per type. The zero value is ready to use; New is
// the conventional constructor.
//
// Instances are keyed by reflect.Type rather than by a rendered type name.
// That keeps the read path free of the string it would otherwise build and
// hash on every lookup, and it is the stricter key: a rendered name is empty
// for unnamed types and uses short package names, so distinct types can
// collide, while reflect.Type values are canonical and compare equal only
// for the same type.
type Store struct {
	// m is a sync.Map for its lock-free read path. Lookups are almost
	// entirely reads — an entry is written once, the first time a value type
	// is cached, and read for the rest of the process — and a shared RWMutex
	// would serialize even those reads on its reader counter, getting slower
	// as cores are added.
	m sync.Map

	// mu serializes creation so create runs once per type. It is never taken
	// on the read path.
	mu sync.Mutex
}

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// Load returns the instance registered under type C, creating it with create
// on first use. Create runs at most once per type.
func Load[C any](s *Store, create func() C) C {
	key := reflect.TypeFor[C]()
	if v, ok := s.m.Load(key); ok {
		return v.(C) //nolint:errcheck
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.m.Load(key); ok {
		return v.(C) //nolint:errcheck
	}
	v := create()
	s.m.Store(key, v)
	return v
}
