package types

import "sync"

// Map represents a generic map interface that can be implemented
// by different map types like HashMap, TreeMap, LinkedHashMap,
// SkipListMap, BTreeMap, etc.
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

	// Clone() Map[K, V]
}

type Locker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

var _ Locker = (*sync.RWMutex)(nil)
