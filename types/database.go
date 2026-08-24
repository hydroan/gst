package types

import "github.com/hydroan/gst/types/consts"

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
	// UpdateByID updates database columns of a record by its ID in one
	// UPDATE statement, without running model hooks. Assignments come from
	// the generated column references (SampleCols.Status.Set(v)) or the
	// Assign constructor for dynamic columns; at least one is required, and
	// empty columns, nil values and a column assigned twice are rejected.
	UpdateByID(id string, assignments ...Assignment) error
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

// DatabaseOption provides chainable options for a single Database operation chain.
// Options apply to the next terminal operation and are reset afterward. Start a
// new chain with database.Database[M](ctx) for each independent operation.
type DatabaseOption[M Model] interface {
	// WithQuery adds query conditions from model fields or raw SQL configuration.
	WithQuery(query M, opts ...QueryOptions) Database[M]
	// WithCursor enables cursor-based pagination for List operations.
	WithCursor(cursor Cursor) Database[M]
	// WithSelect specifies columns for SELECT and Update column selection
	// where supported, through the generated column references.
	WithSelect(columns ...AnyColumnRef) Database[M]
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
	// WithOrder adds ORDER BY terms to sort query results.
	WithOrder(orders ...Order) Database[M]
	// WithExpand enables eager loading of specified associations.
	WithExpand(expand []string, orders ...Order) Database[M]
	// WithPurge controls whether Delete permanently removes records instead of soft deleting them.
	WithPurge(...bool) Database[M]
	// WithDeleted includes soft-deleted records in read operations (List, Get,
	// First, Last, Take, Count). Only the soft-delete condition is lifted;
	// combining it with a write operation or Cleanup fails the chain.
	WithDeleted() Database[M]
	// WithBuildSQL builds SQL for the next terminal operation and appends Query, Args, and RenderedSQL to the collector.
	WithBuildSQL(statements *[]SQLStatement) Database[M]
	// WithDryRun builds SQL without database I/O, framework hooks, cache mutation, or object field filling.
	WithDryRun() Database[M]
	// WithoutHook disables model hooks for the operation.
	WithoutHook() Database[M]
}

// QueryOptions tunes how WithQuery turns a model value into WHERE conditions.
// Every condition it produces is AND-combined; the zero value means exact
// matching with the empty-query safety check enabled. See the WithQuery method
// for usage examples.
type QueryOptions struct {
	// AllowEmpty allows a query without any condition to match all records.
	// By default a nil model, a zero-value model, or all-empty field values
	// add the "1 = 0" safety condition instead, so a forgotten filter cannot
	// return or delete the whole table. RawQuery and Filters count as
	// real conditions and disable the safety check on their own.
	AllowEmpty bool

	// RawQuery is a raw parameterized SQL fragment added as an extra WHERE
	// condition. It works with a nil model and combines with model-field
	// conditions otherwise.
	RawQuery string

	// RawQueryArgs are the values bound to the RawQuery placeholders.
	RawQueryArgs []any

	// PresentFields marks columns whose filter values were explicitly provided
	// by the caller, keyed by snake case column name. Query construction treats
	// zero values (false, 0) of these columns as real conditions instead of
	// dropping them as unset, so a filter like "enabled=false" works. Columns
	// not listed here keep the default zero-value skip.
	PresentFields map[string]struct{}

	// Filters are field-level operator filters ("field[op]=value"). They
	// apply in every WithQuery path, including nil/empty model queries, so
	// List and Count stay consistent. A condition with an unknown operator
	// or empty column fails closed: query construction adds "1 = 0" instead
	// of dropping it.
	Filters []Filter
}

// SQLStatement contains a generated SQL statement in executable and rendered forms.
type SQLStatement struct {
	// Query is the parameterized SQL with placeholders.
	Query string
	// Args contains the values bound to Query.
	Args []any
	// RenderedSQL is dialect-rendered SQL for logging, inspection, and manual debugging.
	RenderedSQL string
}
