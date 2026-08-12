package types

import (
	"context"
	"time"

	"go.uber.org/zap/zapcore"
)

// Model defines the framework contract for database-backed and action models.
// Typical database resources embed model.Base (UUIDv7 string primary key) or
// model.AutoBase (auto-increment integer primary key). Action-only models may
// use model.Empty when they do not represent persistent rows.
//
// Type Requirements:
//   - Must be a pointer to struct (e.g., *User)
//   - Database resources should expose an ID primary key through GetID/SetID/ClearID
//   - Hooks should be idempotent enough to run as part of framework CRUD phases
type Model interface {
	GetTableName() string // GetTableName returns the table name.
	GetID() string        // GetID returns the string form of the id, or "" when the id is unset.
	SetID(id ...string)   // SetID sets the id when unset; Base generates a UUID without an argument while AutoBase leaves generation to the database.
	ClearID()             // ClearID always set the id to empty.
	GetCreatedBy() string
	GetUpdatedBy() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	SetCreatedBy(string)
	SetUpdatedBy(string)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	Expands() []string                            // Expands returns association paths that should be preloaded by default.
	Purge() bool                                  // Purge indicates whether to permanently delete records (hard delete). Default is false (soft delete).
	MarshalLogObject(zapcore.ObjectEncoder) error // MarshalLogObject implements zap.ObjectMarshaler.

	CreateBefore(context.Context) error
	CreateAfter(context.Context) error
	DeleteBefore(context.Context) error
	DeleteAfter(context.Context) error
	UpdateBefore(context.Context) error
	UpdateAfter(context.Context) error
	ListBefore(context.Context) error
	ListAfter(context.Context) error
	GetBefore(context.Context) error
	GetAfter(context.Context) error
}
