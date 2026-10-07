package gst

import "github.com/hydroan/gst/internal/types"

// Model defines the framework contract for database-backed and action models.
// Typical database resources embed model.Base (UUIDv7 string primary key) or
// model.AutoBase (auto-increment integer primary key). Action-only models may
// use model.Empty when they do not represent persistent rows.
type Model = types.Model

// ESDocumenter represents a document that can be indexed into Elasticsearch.
// Types implementing this interface should be able to convert themselves
// into a document format suitable for Elasticsearch indexing.
type ESDocumenter = types.ESDocumenter

// Request and Response are the framework-facing types of one action's request
// and response payloads. They constrain the REQ and RSP type parameters of
// Service and Module; the concrete types are declared per action by the model
// layer.
type (
	Request = types.Request

	Response = types.Response
)

// Service defines the controller-facing business operation contract for a model.
// Generated controllers call these methods for CRUD, batch CRUD, lifecycle hooks,
// import/export, filtering, and logging.
type Service[M Model, REQ Request, RSP Response] = types.Service[M, REQ, RSP]

// Module describes a registered API module: route metadata, auth exposure,
// resource parameter name, and the service implementation used by controllers.
type Module[M Model, REQ Request, RSP Response] = types.Module[M, REQ, RSP]

// ControllerConfig customizes how router.Register builds an internal handler for
// a route. It is the public configuration surface for controller behavior; the
// concrete controller handlers and their runtime state remain framework-owned.
type ControllerConfig[M Model] = types.ControllerConfig[M]

// Database defines the model-scoped database operation contract.
// It provides CRUD operations, query builders, and optional dry-run behavior
// for a single Model type.
type Database[M Model] = types.Database[M]

// DatabaseOption provides chainable options for a single Database operation chain.
// Options apply to the next terminal operation and are reset afterward. Start a
// new chain with database.Database[M](ctx) for each independent operation.
type DatabaseOption[M Model] = types.DatabaseOption[M]

// QueryOptions tunes how WithQuery turns a model value into WHERE conditions.
// Every condition it produces is AND-combined; the zero value means exact
// matching with the empty-query safety check enabled. See the WithQuery method
// for usage examples.
type QueryOptions = types.QueryOptions

// SQLStatement contains a generated SQL statement in executable and rendered forms.
type SQLStatement = types.SQLStatement

// Selector runs an analytical read over the table of M and scans the result
// rows into R. It is deliberately separate from Database[M]: a projected row
// is not a model row, so model hooks, association preloading and cursor
// pagination have nothing to act on and are absent here rather than present
// and inert.
type Selector[M Model, R any] = types.Selector[M, R]

// ErrEntryNotFound is returned when a cache entry is not found, or when the
// stored value cannot be decoded as the handle's type and the entry is dropped.
var ErrEntryNotFound = types.ErrEntryNotFound

// ErrTTLNotSupported is returned by Cache.Set when the backend cannot honor
// the requested ttl semantics, such as a per-entry lifetime on a backend
// without per-entry expiration.
var ErrTTLNotSupported = types.ErrTTLNotSupported

// Cache provides a typed key/value cache abstraction.
type Cache[T any] = types.Cache[T]

// Permission is one operation a role is allowed to perform on one object. It is
// the unit the whole-set replacement methods on RBAC take, so a caller states a
// role's permissions as a set rather than as a sequence of grants.
type Permission = types.Permission

// Decision is the outcome of one authorization check.
type Decision = types.Decision

// RBAC provides tenant-scoped role, permission, and subject assignment operations.
// A process holding no policy set — RBAC disabled, or not initialized — answers
// reads as the deployment they describe, denying every request and reporting no
// roles, and refuses every write rather than reporting a change it did not make.
type RBAC = types.RBAC

// Logger is the logger the framework hands to services and modules and keeps
// in the logger package's streams. It writes an entry plain and printf-style,
// sugared with key/value fields (the "w" methods) and with typed zap.Field
// values (the "z" methods); With attaches string key/value fields and
// WithContext derives a logger carrying request metadata fields.
type Logger = types.Logger
