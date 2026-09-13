package types

import (
	"context"
	"time"
)

// Model defines the framework contract for database-backed and action models.
// Typical database resources embed model.Base (UUIDv7 string primary key) or
// model.AutoBase (auto-increment integer primary key). Action-only models may
// use model.Empty when they do not represent persistent rows.
//
// Type Requirements:
//   - Must be a pointer to struct (e.g., *User)
//   - Database resources should expose an ID primary key through GetID/SetID/ClearID
//   - Database resources must override TableName with an explicit non-empty
//     name: gorm's Tabler reads the same method, and the base default "" is
//     rejected at table preparation and inside gg migrate
//   - Hooks should be idempotent enough to run as part of framework CRUD phases
type Model interface {
	TableName() string  // TableName returns the explicit table name; gorm's Tabler reads the same method.
	GetID() string      // GetID returns the string form of the id, or "" when the id is unset.
	SetID(id ...string) // SetID sets the id when unset; Base generates a UUID without an argument while AutoBase leaves generation to the database.
	ClearID()           // ClearID always sets the id to empty.
	GetCreatedBy() string
	GetUpdatedBy() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	SetCreatedBy(string)
	SetUpdatedBy(string)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	Expands() []string // Expands returns association paths that should be preloaded by default.
	Purge() bool       // Purge indicates whether to permanently delete records (hard delete). Default is false (soft delete).

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

// ESDocumenter represents a document that can be indexed into Elasticsearch.
// Types implementing this interface should be able to convert themselves
// into a document format suitable for Elasticsearch indexing.
type ESDocumenter interface {
	// Document returns a map representing an Elasticsearch document.
	// The returned map should contain all fields to be indexed, where:
	//   - keys are field names (string type)
	//   - values are field values (any type)
	//
	// Implementation notes:
	//   1. The returned map should only contain JSON-serializable values.
	//   2. Field names should match those defined in the Elasticsearch mapping.
	//   3. Complex types (like nested objects or arrays) should be correctly
	//      represented in the returned map.
	//
	// Example:
	//   return map[string]any{
	//       "id":    "1234",
	//       "title": "Sample Document",
	//       "tags":  []string{"tag1", "tag2"},
	//   }
	Document() map[string]any

	// GetID returns a string that uniquely identifies the document.
	// This ID is typically used as the Elasticsearch document ID.
	//
	// Implementation notes:
	//   1. The ID should be unique within the index.
	//   2. If no custom ID is needed, consider returning an empty string
	//      to let Elasticsearch auto-generate an ID.
	//   3. The ID should be a string, even if it's originally a numeric value.
	//
	// Example:
	//   return "user_12345"
	GetID() string
}
