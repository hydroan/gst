// Package types holds what the data structure packages under ds share: the
// iterator and container interfaces they implement, the locker a structure
// guards itself with, and the errors they return.
package types

var _ Locker = FakeLocker{}

type FakeLocker struct{}

func (FakeLocker) Lock()    {}
func (FakeLocker) Unlock()  {}
func (FakeLocker) RLock()   {}
func (FakeLocker) RUnlock() {}
