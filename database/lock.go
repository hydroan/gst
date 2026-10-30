package database

import "github.com/hydroan/gst/internal/types"

// LockMode is the row lock a query asks for through WithLock: FOR UPDATE,
// FOR SHARE, and their NOWAIT and SKIP LOCKED forms.
type LockMode = types.LockMode

// The row lock modes.
const (
	LockUpdate           = types.LockUpdate
	LockShare            = types.LockShare
	LockUpdateNoWait     = types.LockUpdateNoWait
	LockShareNoWait      = types.LockShareNoWait
	LockUpdateSkipLocked = types.LockUpdateSkipLocked
	LockShareSkipLocked  = types.LockShareSkipLocked
)
