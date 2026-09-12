package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Permission is one operation a role is allowed to perform on one object. It is
// the unit the whole-set replacement methods on RBAC take, so a caller states a
// role's permissions as a set rather than as a sequence of grants.
type Permission = itypes.Permission

// Decision is the outcome of one authorization check.
type Decision = itypes.Decision

// RBAC provides tenant-scoped role, permission, and subject assignment operations.
// A process holding no policy set — RBAC disabled, or not initialized — answers
// reads as the deployment they describe, denying every request and reporting no
// roles, and refuses every write rather than reporting a change it did not make.
type RBAC = itypes.RBAC
