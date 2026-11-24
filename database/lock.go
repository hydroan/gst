package database

import "github.com/hydroan/gst/internal/gst"

// LockMode is the row lock a query asks for through WithLock: FOR UPDATE,
// FOR SHARE, and their NOWAIT and SKIP LOCKED forms.
type LockMode = gst.LockMode

// The row lock modes.
const (
	LockUpdate           = gst.LockUpdate
	LockShare            = gst.LockShare
	LockUpdateNoWait     = gst.LockUpdateNoWait
	LockShareNoWait      = gst.LockShareNoWait
	LockUpdateSkipLocked = gst.LockUpdateSkipLocked
	LockShareSkipLocked  = gst.LockShareSkipLocked
)
