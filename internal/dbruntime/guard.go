package dbruntime

import (
	"context"

	"gorm.io/gorm"
)

// transactionGuard is the check every transaction runs on itself before its
// first business statement. One package installs it, from its init: the
// lease engine, so a transaction opened under a lease refuses to run once
// the lease is lost. A project that links no such package has no guard, and
// GuardTransaction costs it a nil check.
var transactionGuard func(ctx context.Context, tx *gorm.DB) error

// SetTransactionGuard installs guard. It is called from a package init
// function, before any transaction opens, which is what lets the slot go
// without a lock.
func SetTransactionGuard(guard func(ctx context.Context, tx *gorm.DB) error) {
	transactionGuard = guard
}

// GuardTransaction runs the installed guard on tx, the transaction ctx is
// about to run business statements on, and returns what it returns; with no
// guard installed it returns nil.
func GuardTransaction(ctx context.Context, tx *gorm.DB) error {
	if transactionGuard == nil {
		return nil
	}
	return transactionGuard(ctx, tx)
}
