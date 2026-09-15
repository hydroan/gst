package dbruntime

import (
	"context"

	"gorm.io/gorm"
)

// transactionGuard is the check every transaction runs on itself before its
// first business statement. One package installs it, from its init: the
// lease engine, so a transaction opened under a lease refuses to run once
// the lease is lost. A project that links no such package has no guard, and
// GuardTransaction costs it a nil check. The guard receives the handle the
// transaction was opened on beside the transaction itself, so it can tell a
// transaction on the primary database from one on another instance.
var transactionGuard func(ctx context.Context, base, tx *gorm.DB) error

// SetTransactionGuard installs guard. It is called from a package init
// function, before any transaction opens, which is what lets the slot go
// without a lock. A second guard panics instead of replacing the first: the
// slot holds one check, and silently swapping it would drop the other
// package's. A nil guard clears the slot, for tests.
func SetTransactionGuard(guard func(ctx context.Context, base, tx *gorm.DB) error) {
	if guard != nil && transactionGuard != nil {
		panic("dbruntime: a transaction guard is already installed")
	}
	transactionGuard = guard
}

// GuardTransaction runs the installed guard on tx, the transaction ctx is
// about to run business statements on and base is the handle it was opened
// on, and returns what it returns; with no guard installed it returns nil.
func GuardTransaction(ctx context.Context, base, tx *gorm.DB) error {
	if transactionGuard == nil {
		return nil
	}
	return transactionGuard(ctx, base, tx)
}
