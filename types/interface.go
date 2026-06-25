package types

import (
	"context"
	"io"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ErrEntryNotFound is returned when a cache entry is not found.
var ErrEntryNotFound = errors.New("cache entry not found")

// Initializer defines a bootstrap component that performs one-time setup.
// Implementations should return an error when required configuration, connections,
// or runtime resources cannot be initialized.
type Initializer interface {
	Init() error
}

// StandardLogger provides plain and printf-style leveled logging methods.
// Fatal and Fatalf follow the underlying logger's fatal behavior and should
// terminate the process after writing the log entry.
type StandardLogger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Fatal(args ...any)

	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// StructuredLogger provides sugared structured logging with alternating
// key/value fields. Methods with suffix "w" mean "with fields".
type StructuredLogger interface {
	Debugw(msg string, keysAndValues ...any)
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
	Fatalw(msg string, keysAndValues ...any)
}

// ZapLogger provides structured logging with typed zap.Field values.
// Methods with suffix "z" are the low-allocation typed-field variants.
type ZapLogger interface {
	Debugz(msg string, fields ...zap.Field)
	Infoz(msg string, fields ...zap.Field)
	Warnz(msg string, fields ...zap.Field)
	Errorz(msg string, fields ...zap.Field)
	Fatalz(msg string, fields ...zap.Field)
}

// Logger combines plain, sugared structured, and typed zap logging methods.
// With attaches string key/value fields. WithObject, WithArray, and the context
// helpers return derived loggers with additional structured fields.
type Logger interface {
	With(fields ...string) Logger

	WithObject(name string, obj zapcore.ObjectMarshaler) Logger
	WithArray(name string, arr zapcore.ArrayMarshaler) Logger

	WithContext(context.Context, consts.Phase) Logger
	WithRequestMetadata(RequestMetadata, consts.Phase) Logger

	StandardLogger
	StructuredLogger
	ZapLogger
}

// Database defines the model-scoped database operation contract.
// It provides CRUD operations, query builders, transactions, cleanup, health checks,
// and optional cache/dry-run behavior for a single Model type.
//
// Type Parameters:
//   - M: Model type that implements Model interface
//
// The interface embeds DatabaseOption[M] to provide chainable query building.
// A chain is expected to end with one terminal operation, such as Create, List,
// Get, Count, Cleanup, Health, Transaction, or TransactionFunc.
//
// Implementations share an underlying GORM session. Call database.Database[M](ctx)
// again for each independent operation chain. Keeping the returned value in a
// variable and running another independent operation on it (for example, List
// then Get or Update) is incorrect usage; see database.Database.
type Database[M Model] interface {
	// Create inserts one or more records, setting framework IDs and timestamps unless WithDryRun is enabled.
	Create(objs ...M) error
	// Delete removes one or more records using WithPurge, the model Purge setting, or soft delete by default.
	Delete(objs ...M) error
	// Update saves one or more full model values and updates timestamps unless WithDryRun is enabled.
	Update(objs ...M) error
	// UpdateByID updates a single field of a record by its ID.
	UpdateByID(id string, key string, value any) error
	// List retrieves multiple records matching the query conditions.
	List(dest *[]M) error
	// Get retrieves a single record by its ID.
	Get(dest M, id string) error
	// First retrieves the first record matching the current query conditions.
	First(dest M) error
	// Last retrieves the last record matching the current query conditions.
	Last(dest M) error
	// Take retrieves the first record in no specified order.
	Take(dest M) error
	// Count returns the total number of records matching the query conditions.
	Count(*int64) error
	// Cleanup permanently deletes all soft-deleted records; WithDryRun only builds the cleanup SQL.
	Cleanup() error
	// Health checks database connectivity and is not disabled by WithDryRun.
	Health() error
	// Transaction executes fn in a transaction for this model and passes a transaction-bound Database.
	Transaction(fn func(txDB Database[M]) error) error
	// TransactionFunc executes fn in a transaction for multi-model work; each Database used inside fn must call WithTx(tx).
	TransactionFunc(fn func(tx any) error) error

	DatabaseOption[M]
}

// DatabaseOption provides chainable options for a single Database operation chain.
// Options apply to the next terminal operation and are reset afterward. Start a
// new chain with database.Database[M](ctx) for each independent operation.
type DatabaseOption[M Model] interface {
	// WithDB uses a custom *gorm.DB; callers must migrate custom schemas explicitly.
	WithDB(any) Database[M]
	// WithTx binds operations to a *gorm.DB transaction, primarily inside TransactionFunc.
	WithTx(tx any) Database[M]
	// WithTable sets a custom table name; the table must already exist.
	WithTable(name string) Database[M]
	// WithDebug enables debug mode to show detailed SQL queries.
	WithDebug() Database[M]
	// WithQuery adds query conditions from model fields or raw SQL configuration.
	WithQuery(query M, config ...QueryConfig) Database[M]
	// WithCursor enables cursor-based pagination for List operations.
	WithCursor(string, bool, ...string) Database[M]
	// WithTimeRange applies a time range filter to the query.
	WithTimeRange(columnName string, startTime time.Time, endTime time.Time) Database[M]
	// WithSelect specifies fields for SELECT and Update column selection where supported.
	WithSelect(columns ...string) Database[M]
	// WithIndex specifies database index hints for query optimization (MySQL only).
	WithIndex(indexName string, hint ...consts.IndexHintMode) Database[M]
	// WithRollback configures a callback that runs when Transaction or TransactionFunc rolls back.
	WithRollback(rollbackFunc func()) Database[M]
	// WithLock adds row-level locking to SELECT queries (must be used within a transaction).
	WithLock(mode ...consts.LockMode) Database[M]
	// WithBatchSize sets the batch size for Create, Update, and Delete.
	WithBatchSize(size int) Database[M]
	// WithPagination applies pagination parameters (page, size) to the query.
	WithPagination(page, size int) Database[M]
	// WithLimit restricts the number of returned records for read operations.
	WithLimit(limit int) Database[M]
	// WithOffset skips records before returning read operation results.
	WithOffset(offset int) Database[M]
	// WithExclude excludes records matching specified conditions.
	WithExclude(map[string][]any) Database[M]
	// WithOrder adds ORDER BY clause to sort query results.
	WithOrder(order string) Database[M]
	// WithExpand enables eager loading of specified associations.
	WithExpand(expand []string, order ...string) Database[M]
	// WithPurge controls whether Delete permanently removes records instead of soft deleting them.
	WithPurge(...bool) Database[M]
	// WithCache enables cache reads/writes for supported read operations and cache invalidation for writes.
	WithCache(...bool) Database[M]
	// WithOmit excludes specified fields from INSERT, UPDATE, and SELECT operations.
	WithOmit(...string) Database[M]
	// WithBuildSQL builds SQL for the next terminal operation and appends Query, Args, and RenderedSQL to the collector.
	WithBuildSQL(statements *[]SQLStatement) Database[M]
	// WithDryRun builds SQL without database I/O, framework hooks, cache mutation, or object field filling.
	WithDryRun() Database[M]
	// WithoutHook disables model hooks for the operation.
	WithoutHook() Database[M]
}

// Model defines the framework contract for database-backed and action models.
// Typical database resources embed model.Base. Action-only models may use
// model.Empty or model.Any when they do not represent persistent rows.
//
// Type Requirements:
//   - Must be a pointer to struct (e.g., *User)
//   - Database resources should expose an ID primary key through GetID/SetID/ClearID
//   - Hooks should be idempotent enough to run as part of framework CRUD phases
type Model interface {
	GetTableName() string // GetTableName returns the table name.
	GetID() string
	SetID(id ...string) // SetID method will automatically set the id if id is empty.
	ClearID()           // ClearID always set the id to empty.
	GetCreatedBy() string
	GetUpdatedBy() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	SetCreatedBy(string)
	SetUpdatedBy(string)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	Expands() []string // Expands returns association paths that should be preloaded by default.
	Excludes() map[string][]any
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

type (
	Request  any
	Response any
)

// Service defines the controller-facing business operation contract for a model.
// Generated controllers call these methods for CRUD, batch CRUD, lifecycle hooks,
// import/export, filtering, and logging.
//
// Type Parameters:
//   - M: Model type that implements Model interface
//   - REQ: Request type for the current action or resource operation
//   - RSP: Response type for the current action or resource operation
//
// Custom actions should use action-specific REQ/RSP types instead of reusing
// types from other endpoints, even when the fields are identical.
type Service[M Model, REQ Request, RSP Response] interface {
	Create(*ServiceContext, REQ) (RSP, error)
	Delete(*ServiceContext, REQ) (RSP, error)
	Update(*ServiceContext, REQ) (RSP, error)
	Patch(*ServiceContext, REQ) (RSP, error)
	List(*ServiceContext, REQ) (RSP, error)
	Get(*ServiceContext, REQ) (RSP, error)

	CreateMany(*ServiceContext, REQ) (RSP, error)
	DeleteMany(*ServiceContext, REQ) (RSP, error)
	UpdateMany(*ServiceContext, REQ) (RSP, error)
	PatchMany(*ServiceContext, REQ) (RSP, error)

	CreateBefore(*ServiceContext, M) error
	CreateAfter(*ServiceContext, M) error
	DeleteBefore(*ServiceContext, M) error
	DeleteAfter(*ServiceContext, M) error
	UpdateBefore(*ServiceContext, M) error
	UpdateAfter(*ServiceContext, M) error
	PatchBefore(*ServiceContext, M) error
	PatchAfter(*ServiceContext, M) error
	ListBefore(*ServiceContext, *[]M) error
	ListAfter(*ServiceContext, *[]M) error
	GetBefore(*ServiceContext, M) error
	GetAfter(*ServiceContext, M) error

	CreateManyBefore(*ServiceContext, ...M) error
	CreateManyAfter(*ServiceContext, ...M) error
	DeleteManyBefore(*ServiceContext, ...M) error
	DeleteManyAfter(*ServiceContext, ...M) error
	UpdateManyBefore(*ServiceContext, ...M) error
	UpdateManyAfter(*ServiceContext, ...M) error
	PatchManyBefore(*ServiceContext, ...M) error
	PatchManyAfter(*ServiceContext, ...M) error

	Import(*ServiceContext, io.Reader) ([]M, error)
	Export(*ServiceContext, ...M) ([]byte, error)

	Filter(*ServiceContext, M) M
	FilterRaw(*ServiceContext) string

	Logger
}

// Cache provides a typed key/value cache abstraction with TTL and context propagation.
//
// Type Parameters:
//   - T: Cached value type
//
// Error Handling:
//   - Get/Peek return ErrEntryNotFound when key doesn't exist
//   - Set/Delete return backend errors when storage operations fail
type Cache[T any] interface {
	// Get retrieves a value from the cache by key.
	// Returns ErrEntryNotFound if the key does not exist.
	Get(key string) (T, error)

	// Peek retrieves a value from the cache by key without affecting its position or access time.
	// Returns ErrEntryNotFound if the key does not exist.
	Peek(key string) (T, error)

	// Set stores a value in the cache with the specified TTL.
	// A zero TTL means the entry will not expire.
	Set(key string, value T, ttl time.Duration) error

	// Delete removes a key from the cache.
	// Returns ErrEntryNotFound if the key does not exist.
	Delete(key string) error

	// Exists checks if a key exists in the cache.
	// Returns true if the key exists, false otherwise.
	Exists(key string) bool

	// Len returns the number of entries currently stored in the cache.
	Len() int

	// Clear removes all entries from the cache.
	Clear()

	// WithContext returns a cache handle that uses ctx for tracing or cancellation propagation.
	//
	// Implementations may return a new handle or mutate and return the receiver.
	// Callers must not assume the returned handle is independent unless a concrete
	// provider documents that stronger guarantee.
	WithContext(ctx context.Context) Cache[T]
}

// DistributedCache extends Cache with explicit local-plus-remote synchronization helpers.
//
// Type Parameters:
//   - T: Cached value type
type DistributedCache[T any] interface {
	Cache[T]

	// SetWithSync stores a value in both local and distributed cache with synchronization.
	SetWithSync(key string, value T, localTTL time.Duration, remoteTTL time.Duration) error

	// GetWithSync retrieves a value from local cache first, then from distributed cache if not found.
	GetWithSync(key string, localTTL time.Duration) (T, error)

	// DeleteWithSync removes a value from both local and distributed cache with synchronization.
	DeleteWithSync(key string) error
}

// RBAC provides role, permission, and subject assignment operations.
// When RBAC is disabled or not initialized, the framework may provide a safe
// no-op implementation whose methods succeed without side effects.
//
// RBAC Model:
//   - Subject: Users or entities that need access
//   - Role: Named collection of permissions
//   - Resource: Protected objects or endpoints
//   - Action: Operations on resources
type RBAC interface {
	AddRole(name string) error
	RemoveRole(name string) error

	GrantPermission(role string, resource string, action string) error
	// RevokePermission removes policies for the given role with flexible behaviors:
	// - resource=="" && action=="" : remove all policies for the role
	// - resource=="" && action!="" : remove policies matching the role and action
	// - resource!="" && action=="" : remove policies matching the role and resource
	// - resource!="" && action!="" : remove the exact (role, resource, action, "allow") policy
	RevokePermission(role string, resource string, action string) error

	AssignRole(subject string, role string) error
	UnassignRole(subject string, role string) error
}

// Module describes a registered API module: route metadata, auth exposure,
// resource parameter name, and the service implementation used by controllers.
//
// Type Parameters:
//   - M: Model type that implements Model interface
//   - REQ: Request type for API operations
//   - RSP: Response type for API operations
//
// Features:
//   - Automatic route registration
//   - Service layer integration
//   - Configurable authentication
type Module[M Model, REQ Request, RSP Response] interface {
	// Service returns the service instance that handles business logic for this module.
	Service() Service[M, REQ, RSP]

	// Route returns the base API path for this module's endpoints.
	Route() string

	// Pub determines whether the API endpoints are public or require authentication.
	Pub() bool

	// Param returns the URL parameter name used for resource identification.
	Param() string
}

// Coder describes an API envelope code, HTTP status, and client-safe message.
type Coder interface {
	Code() int
	Status() int
	Msg() string
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
