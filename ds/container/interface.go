package container

import "sync"

// Map is the interface of a keyed container: reading, writing and removing
// the value of a key, and walking the keys and the values.
type Map[K, V any] interface {
	Get(key K) (V, bool)
	Set(key K, value V)
	Remove(key K) bool
	Has(key K) bool

	Clear()
	Len() int
	IsEmpty() bool

	Keys() []K
	Values() []V
	Range(func(key K, value V) bool)
}

// Locker is the lock a structure holds: a sync.RWMutex once WithSafe is
// given, and in most structures a NopLocker until then.
type Locker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

var _ Locker = (*sync.RWMutex)(nil)
