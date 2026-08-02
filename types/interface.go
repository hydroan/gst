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
// Get, Count, Cleanup, or Health.
//
// Implementations share an underlying GORM session. Call database.Database[M](ctx)
// again for each independent operation chain. Keeping the returned value in a
// variable and running another independent operation on it (for example, List
// then Get or Update) is incorrect usage; see database.Database.
//
// Transactions are started with the package-level database.Transaction
// function; the context it passes to fn makes every chain started from that
// context join the transaction automatically.
type Database[M Model] interface {
	// Create inserts one or more records (pure INSERT), setting framework IDs
	// and forcing created_at/updated_at to now. A primary or unique key
	// collision fails with database.ErrDuplicatedKey instead of updating the
	// existing row.
	Create(objs ...M) error
	// Delete removes one or more records using WithPurge, the model Purge setting, or soft delete by default.
	Delete(objs ...M) error
	// Update saves one or more full model values by primary key (pure UPDATE,
	// zero values included). Objects without an ID fail with
	// database.ErrIDRequired; records without a live row fail with
	// database.ErrRecordNotFound. created_at/created_by/deleted_at are never
	// written; updated_at is always refreshed by the framework.
	Update(objs ...M) error
	// Upsert inserts records or, on any unique-key collision, overwrites the
	// conflicting row (INSERT ... ON DUPLICATE KEY UPDATE). It runs no model
	// hooks and re-syncs caller objects with the persisted rows; reserve it
	// for deliberate merge writes such as imports and sync jobs.
	Upsert(objs ...M) error
	// UpdateByID updates a single database column of a record by its ID.
	UpdateByID(id string, column string, value any) error
	// List retrieves multiple records matching the query conditions.
	// dest must be a non-nil pointer to a slice; the slice value itself may be
	// nil or preallocated with make. List fully replaces the slice contents with
	// the query result: any pre-existing elements are discarded, never merged or
	// appended to. After a successful call len(*dest) equals the number of rows
	// returned, so a "dirty" dest does not leak stale rows into the result.
	List(dest *[]M) error
	// Get retrieves a single record by its ID.
	// The destination must be a non-nil pointer matching M. When M is *T,
	// both &value and new(T) are valid destinations; a nil *T returns ErrNilDest.
	// Get returns database.ErrRecordNotFound when no matching record exists.
	Get(dest M, id string) error
	// First retrieves the first record matching the current query conditions.
	// First returns database.ErrRecordNotFound when no matching record exists.
	First(dest M) error
	// Last retrieves the last record matching the current query conditions.
	// Last returns database.ErrRecordNotFound when no matching record exists.
	Last(dest M) error
	// Take retrieves the first record in no specified order.
	// Take returns database.ErrRecordNotFound when no matching record exists.
	Take(dest M) error
	// Count returns the total number of records matching the query conditions.
	Count(*int) error
	// Cleanup permanently deletes all soft-deleted records; WithDryRun only builds the cleanup SQL.
	Cleanup() error
	// Health checks database connectivity and is not disabled by WithDryRun.
	Health() error

	DatabaseOption[M]
}

// Aggregator runs an analytical read over the table of M and scans the result
// rows into R. It is deliberately separate from Database[M]: an aggregate
// result is not a model row, so model hooks, association preloading and cursor
// pagination have nothing to act on and are absent here rather than present
// and inert.
//
// Scoping comes from M — the table name, the soft-delete condition and the
// dialect — so an aggregate can never read rows a List on the same model
// hides. R is an ordinary struct the caller declares; its fields bind to the
// projection aliases, and a mismatch on either side is a build error rather
// than a silently zero column. A measure that can come back NULL — AVG, MIN
// or MAX without group keys, carrying conditions, or over a nullable column —
// must bind to a pointer or sql.Null field, which is again a build error
// rather than a zero on the report.
//
// The entry point is the package-level database.Aggregate[M, R] rather than a
// method, because a Go method cannot introduce the result type parameter.
//
// Row-level access rules are not inherited. A model's List gets its tenant or
// group scoping from the Filter service hook the controller runs;
// an aggregate is called straight from service code, so those hooks never run
// and every scoping condition has to be passed to Where explicitly. Forgetting
// one aggregates across tenants without any sign that it did.
//
// A builder is a specification, not a live statement: it can be read more than
// once, and each terminal renders the spec afresh. That is what makes the
// paginated-report idiom safe -- Scan for the page, then CountGroups for the
// total, off the same builder.
//
// Example:
//
//	type tenantTotal struct {
//	    TenantID string
//	    Total    int64
//	    // ClosedAt is nullable on Sample, so MAX over it can be NULL and
//	    // needs a field that can hold NULL.
//	    LastClosed *time.Time
//	}
//	total := SampleCols.Amount.Sum().As("total")
//	rows := make([]tenantTotal, 0)
//	err := database.Aggregate[*Sample, tenantTotal](ctx).
//	    Select(SampleCols.TenantID.Group(), total,
//	        SampleCols.ClosedAt.Max().As("last_closed")).
//	    Where(SampleCols.Status.Eq(StatusDone)).
//	    Having(total.Gte(1000)).
//	    OrderBy(total.Desc()).
//	    Limit(10).
//	    Scan(&rows)
type Aggregator[M Model, R any] interface {
	// Select declares the projection. A term without an aggregate function is
	// a group key, and GROUP BY is derived from those keys, so the SELECT and
	// GROUP BY lists cannot disagree. At least one aggregate term is required.
	Select(terms ...AggregateTerm) Aggregator[M, R]
	// Where restricts the rows entering the aggregation, using the same filter
	// tree as WithQuery.
	Where(filters ...Filter) Aggregator[M, R]
	// Having restricts the produced groups by their measures.
	Having(conditions ...Having) Aggregator[M, R]
	// OrderBy sorts the result rows by a projection term.
	OrderBy(orders ...AggregateOrder) Aggregator[M, R]
	// Limit caps the number of result rows.
	Limit(n int) Aggregator[M, R]
	// Offset skips result rows, for paginating a grouped report.
	Offset(n int) Aggregator[M, R]

	// Scan runs the query and fills dest with one element per group.
	Scan(dest *[]R) error
	// ScanOne runs an ungrouped aggregation and fills dest with its single
	// row. It fails when the projection declares group keys.
	ScanOne(dest *R) error
	// CountGroups reports how many groups the query produces, which is the
	// total a paginated grouped report needs.
	CountGroups(count *int) error

	// WithBuildSQL builds the SQL for the next terminal operation and appends
	// it to the collector instead of executing it.
	WithBuildSQL(statements *[]SQLStatement) Aggregator[M, R]
	// WithDryRun builds the SQL without database I/O.
	WithDryRun() Aggregator[M, R]
}

// DatabaseOption provides chainable options for a single Database operation chain.
// Options apply to the next terminal operation and are reset afterward. Start a
// new chain with database.Database[M](ctx) for each independent operation.
type DatabaseOption[M Model] interface {
	// WithQuery adds query conditions from model fields or raw SQL configuration.
	WithQuery(query M, opts ...QueryOptions) Database[M]
	// WithCursor enables cursor-based pagination for List operations.
	WithCursor(cursor Cursor) Database[M]
	// WithSelect specifies fields for SELECT and Update column selection where supported.
	WithSelect(columns ...string) Database[M]
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
	// WithOrder adds ORDER BY terms to sort query results.
	WithOrder(orders ...Order) Database[M]
	// WithExpand enables eager loading of specified associations.
	WithExpand(expand []string, orders ...Order) Database[M]
	// WithPurge controls whether Delete permanently removes records instead of soft deleting them.
	WithPurge(...bool) Database[M]
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
//
// Nil-safety contract: when invoked by the generated controllers, ctx is
// never nil and req is never a nil pointer — the controller constructs a
// fresh *ServiceContext per call and instantiates REQ via reflection before
// binding, so implementations do not need defensive nil checks on ctx or req.
//
// Non-nil does not mean populated: List/Get never bind a request body, and
// Create/Update tolerate an empty body, so req may point to a zero-value
// struct. Validate required business fields instead of checking for nil.
//
// The contract only covers framework-invoked calls. Code that calls a
// service method directly (tests, jobs, or code bypassing the controller
// layer) must supply non-nil arguments itself.
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

	// Filter lets a service rewrite the query condition before the
	// controller-side listing runs (List and Export). The model carries the
	// URL-decoded equality condition and the options carry the parsed operator
	// filters; the typical use is row-level data scoping: append typed filters
	// (e.g. Cols.GroupID.In(...)) to options.Filters or narrow the model
	// condition, then return both. Returning an error aborts the request — the
	// correct behavior when loading the caller's data scope fails. The
	// controller calls Filter once and shares the result between List and
	// Count, so both always see the same condition set.
	Filter(*ServiceContext, M, QueryOptions) (M, QueryOptions, error)

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

// Decision is the outcome of one authorization check.
//
// Source names the strongest rule that allowed the request and is empty on a
// denial, because a denial has no granting rule. MatchedRule is the policy row
// that allowed it, and is nil unless Source names a policy: the rules that
// allow without consulting one leave the engine free to report an unrelated
// row, which would read as the reason for access while being nothing of the
// kind.
type Decision struct {
	Allowed     bool
	Source      consts.GrantSource
	MatchedRule []string
}

// RBAC provides tenant-scoped role, permission, and subject assignment operations.
// When RBAC is disabled or not initialized, the framework may provide a safe
// no-op implementation whose methods succeed without side effects.
//
// RBAC Model:
//   - Tenant: Authorization domain for roles, permissions, and assignments
//   - Subject: Users or entities that need access
//   - Role: Named collection of permissions
//   - Object: Protected resources or endpoints
//   - Action: Operations on resources
type RBAC interface {
	// Authorize reports whether subject may perform action on object inside
	// tenant, and what allowed it.
	//
	// Implementations should treat tenant as the authorization domain, subject as
	// the authenticated identity, object as the protected route or resource, and
	// action as the operation being checked, such as an HTTP method.
	//
	// The reason is answered alongside the decision rather than by a second
	// method. Deriving it costs a handful of allocations against the thousands
	// the decision itself takes, so a decision-only entry point would be a
	// second way to ask one question, distinguished by a saving too small to
	// measure.
	Authorize(ctx context.Context, tenant string, subject string, object string, action string) (Decision, error)

	// RemoveRole removes role from tenant, including its permission policies and
	// subject assignments. Callers should use this when deleting a role record so
	// authorization state does not retain stale grants.
	RemoveRole(ctx context.Context, tenant string, role string) error

	// SetRolePermissions replaces the entire permission set held by role inside
	// tenant with permissions, leaving the role's subject assignments untouched.
	//
	// It replaces rather than adds on purpose: the argument is the whole truth,
	// so an entry the caller drops stops allowing requests, and passing an empty
	// set revokes everything. A grant-only API would leave a removed entry
	// allowing requests forever, with nothing left in the source to show it.
	//
	// Implementations must apply the whole set as one step. Revoking and then
	// granting back one permission at a time exposes the role's members to an
	// empty or partial set while the replacement is in flight, which denies
	// requests the role is entitled to.
	//
	// It is the only way to write a role's permissions, which is why it takes
	// the whole set: an interface offering a single grant beside it would let a
	// caller build one up a row at a time and never learn that the entry it
	// dropped is still allowing requests.
	SetRolePermissions(ctx context.Context, tenant string, role string, permissions []Permission) error

	// SetPermissionsForAuthenticated replaces the entire set of permissions every
	// authenticated subject holds. The grant is bound to neither a tenant nor a
	// role, so it reaches subjects that hold no role at all, in every tenant.
	//
	// It is SetRolePermissions for the implicit role every authenticated subject
	// carries, and shares its contract: the argument is the whole truth, an empty
	// set revokes everything, and the whole set is applied as one step.
	//
	// Reserve it for objects that answer only about the caller and already narrow
	// their result to what the caller may see; anything else granted this way
	// becomes reachable by every subject that can log in. Unauthenticated requests
	// are unaffected, because authorization runs only after authentication.
	SetPermissionsForAuthenticated(ctx context.Context, permissions []Permission) error

	// AssignRole assigns subject to role inside tenant.
	// This creates tenant membership for subject and makes the role's
	// tenant-scoped permissions available to that subject.
	AssignRole(ctx context.Context, tenant string, subject string, role string) error

	// UnassignRole removes subject's assignment to role inside tenant.
	// Other roles held by the same subject in the same tenant are left unchanged.
	UnassignRole(ctx context.Context, tenant string, subject string, role string) error

	// RolesForSubject returns the roles subject holds inside tenant.
	//
	// It answers both questions the pair it replaced answered separately:
	// membership is a non-empty result, and holding one particular role is that
	// role being among them. Neither deserved an entry point of its own, and
	// keeping the general one leaves this and SubjectsInTenant as the two
	// directions of a single relation.
	RolesForSubject(ctx context.Context, tenant string, subject string) ([]string, error)

	// SubjectsInTenant returns subjects with at least one role assignment in
	// tenant. It checks membership, not whether any specific route is authorized.
	SubjectsInTenant(ctx context.Context, tenant string) ([]string, error)

	// AssignSystemRole assigns subject to a system-level role outside any tenant.
	// System roles are intended for cross-tenant framework privileges and should
	// not be used for ordinary tenant-local authorization.
	AssignSystemRole(ctx context.Context, subject string, role string) error

	// UnassignSystemRole removes subject's assignment to a system-level role.
	UnassignSystemRole(ctx context.Context, subject string, role string) error

	// HasSystemRole reports whether subject holds a system-level role.
	// This check is separate from Authorize because system roles are not scoped to
	// tenant route policies.
	HasSystemRole(ctx context.Context, subject string, role string) (bool, error)

	// RemoveSubject removes every role assignment held by subject, both
	// tenant-scoped and system-level, across all tenants. Use this when a
	// subject is deleted or deactivated so no orphaned role bindings remain.
	RemoveSubject(ctx context.Context, subject string) error

	// ReloadPolicies discards the authorization state the process holds in
	// memory and rebuilds it from storage.
	//
	// Implementations answer from memory and keep it in step as they write, so
	// the two agree as long as this process is the only writer. They stop
	// agreeing when the stored rules change behind its back: another replica
	// writing them, an operator repairing them by hand, a restore. Nothing
	// detects that on its own, so this is the lever that puts a process back
	// onto the stored state without restarting it.
	//
	// It reads every rule and is not part of the write path, which maintains
	// memory itself. Reserve it for recovery and for the moment a change is
	// known to have happened elsewhere.
	ReloadPolicies(ctx context.Context) error
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
