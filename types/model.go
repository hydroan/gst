package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Model defines the framework contract for database-backed and action models.
// Typical database resources embed model.Base (UUIDv7 string primary key) or
// model.AutoBase (auto-increment integer primary key). Action-only models may
// use model.Empty when they do not represent persistent rows.
type Model = itypes.Model
