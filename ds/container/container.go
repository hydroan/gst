// Package container holds what the data structure packages under ds share:
// the Locker a structure guards itself with and the errors their
// constructors and methods return. It also declares the iterator interfaces,
// which the iterator of the linked list implements, and Map, the interface
// of a keyed container, which no structure under ds implements.
package container

var _ Locker = NopLocker{}

// NopLocker is a Locker whose methods do nothing. Most structures hold one
// until WithSafe replaces it with a sync.RWMutex, so a structure that locks
// without asking how it was built, the linked list for one, runs unguarded.
type NopLocker struct{}

func (NopLocker) Lock()    {}
func (NopLocker) Unlock()  {}
func (NopLocker) RLock()   {}
func (NopLocker) RUnlock() {}
