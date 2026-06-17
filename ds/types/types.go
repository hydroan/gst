package types

var _ Locker = FakeLocker{}

type FakeLocker struct{}

func (FakeLocker) Lock()    {}
func (FakeLocker) Unlock()  {}
func (FakeLocker) RLock()   {}
func (FakeLocker) RUnlock() {}
