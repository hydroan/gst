package database

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

var (
	// ErrInvalidDB reports an operation chain running on a database handle
	// that was never initialized.
	ErrInvalidDB = errors.New("invalid database, maybe not initialized")

	// ErrNilCount is returned when Count or CountGroups is handed a nil
	// destination.
	ErrNilCount = errors.New("count parameter cannot be nil")

	// ErrNilDest is returned when a read operation is handed a nil
	// destination.
	ErrNilDest = errors.New("dest parameter cannot be nil")

	// ErrEmptyFieldName is returned when UpdateByID is handed an empty column
	// name.
	ErrEmptyFieldName = errors.New("field name cannot be empty")

	// ErrNilValue is returned when UpdateByID is handed a nil value.
	ErrNilValue = errors.New("value cannot be nil")

	// ErrNoAssignments is returned when UpdateByID is called without any
	// assignment.
	ErrNoAssignments = errors.New("update requires at least one assignment")

	// ErrDuplicateColumn is returned when one UpdateByID call assigns the
	// same column twice.
	ErrDuplicateColumn = errors.New("column is assigned twice in one update")

	// ErrIDRequired is returned when an operation that addresses records by
	// primary key is handed a record or an id argument without one.
	ErrIDRequired = errors.New("id is required")

	// ErrRecordNotFound is the gorm sentinel for a read that matches no live
	// row; Get and Update also answer it for a missing or soft-deleted record.
	ErrRecordNotFound = gorm.ErrRecordNotFound

	// ErrDuplicatedKey is the gorm sentinel for a write colliding with a
	// primary or unique key, translated from the dialect's own error.
	ErrDuplicatedKey = gorm.ErrDuplicatedKey

	// ErrNilSQLBuilder is returned when WithBuildSQL runs without a statement
	// collector to append to.
	ErrNilSQLBuilder = errors.New("sql statement collector cannot be nil")

	// ErrNilTransaction is returned when Transaction or TransactionOn is
	// handed a nil closure.
	ErrNilTransaction = errors.New("transaction function cannot be nil")

	// ErrUnusableFilter reports a filter the renderer cannot apply. Client
	// query paths fail closed to an empty result and only log it; server-built
	// readers such as the aggregate builder surface it instead, because there
	// an unusable predicate would disguise a bug as "no data".
	ErrUnusableFilter = errors.New("filter cannot be applied")

	// ErrTransactionInstance is returned when the instance handed to DatabaseOn
	// or TransactionOn is itself an open transaction rather than the connection
	// it was opened on.
	ErrTransactionInstance = errors.New("database instance is an open transaction: pass the instance it was opened on")

	// ErrUnsupportedOnDialect is returned when an operation is invoked on a
	// dialect that does not carry it, per the capability-miss rule: the entry
	// fails instead of silently degrading. Today that is Upsert, Cleanup, the
	// transaction boundary, and row locks on a ClickHouse instance; each entry
	// point states its own reason.
	ErrUnsupportedOnDialect = errors.New("operation is not supported on this dialect")

	// ErrLockOutsideTransaction is returned when WithLock is used on a chain
	// that is not inside a transaction, where the lock it asks for would be
	// released as soon as the statement finished.
	ErrLockOutsideTransaction = errors.New("WithLock requires a transaction: wrap the operation in database.Transaction")

	// ErrAfterCommit marks a failure that happened after the transaction
	// committed. Callers distinguish it with errors.Is because the two outcomes
	// call for opposite handling: an ordinary error means the write was rolled
	// back and nothing happened, while this one means the write is durable and
	// only a follow-up effect failed.
	ErrAfterCommit = errors.New("after-commit action failed")
)

var (
	defaultLimit           = -1
	defaultBatchSize       = 1000
	defaultDeleteBatchSize = 10000
	defaultsColumns        = []string{
		"id",
		"created_by",
		"updated_by",
		"created_at",
		"updated_at",
		"deleted_at",
	}
)

// DB returns the framework-managed default GORM database handle.
//
// The returned handle exposes the current runtime connection for advanced
// integrations, but framework initialization owns the underlying pointer.
// Callers should use Database[M](ctx) for normal CRUD operations.
//
// Running SQL directly on this raw handle bypasses everything Database[M](ctx)
// wires up per request, so statements issued here are invisible to the tools
// used for troubleshooting:
//
//   - Log correlation: the GORM SQL logger reads trace_id, user_id, and
//     username from the statement context. The raw handle carries no request
//     context, so its SQL log entries have empty trace/user fields and can
//     never be joined with access/controller/service logs when tracing a
//     request by trace_id.
//   - Tracing: GormTracingPlugin parents each SQL span on the statement
//     context. Statements on the raw handle produce orphan root spans outside
//     the request trace, and with parent-based sampling they may not be
//     recorded at all.
//   - Transaction propagation: Database[M](ctx) joins the transaction carried
//     by ctx (database.Transaction or a model-hook write). The raw handle
//     always talks to the root connection, so its writes silently escape the
//     surrounding transaction and break its all-or-nothing guarantee.
//
// When the raw handle is unavoidable (maintenance SQL, schema tweaks in
// tests), pass the request context along: DB().WithContext(ctx) restores log
// correlation and span parenting. Transaction propagation still requires
// Database[M](ctx) — a context-carried transaction is never visible to the
// raw handle.
func DB() *gorm.DB {
	return dbruntime.DB
}

// database implements types.Database[M].
type database[M types.Model] struct {
	ins *gorm.DB
	m   M
	typ reflect.Type
	ctx context.Context
	mu  sync.Mutex

	// identity
	base *gorm.DB // connection handle this chain was opened on; it keys context transactions and stays the original handle even after the chain joins one. Set at entry, never reset.

	// err is a defect in how this chain was built, reported by whichever
	// terminal operation runs first. It is deliberately not cleared by reset:
	// the chain is invalid for its whole life, not just for one operation.
	err error

	// options
	enablePurge *bool // delete resource permanently, not only update deleted_at field, only works on 'Delete' method.
	batchSize   int   // batch size for bulk operations. affects Create, Update, Delete.
	noHook      bool  // disable model hook.
	dryRun      bool  // build SQL without database I/O, hooks, or object field filling.

	// sql
	buildingSQL   bool // collect generated SQL statements for WithBuildSQL.
	sqlStatements *[]types.SQLStatement

	// cursor pagination
	cursor types.Cursor // feed ordering, boundary value, and travel direction; a zero Value disables cursor pagination.

	// select
	selectColumns []string
}

func (db *database[M]) quoteIdent(name string) string {
	if db == nil || db.ins == nil || db.ins.Statement == nil {
		return name
	}
	return db.ins.Statement.Quote(name)
}

func (db *database[M]) quoteTableColumn(table, column string) string {
	if len(table) == 0 {
		return db.quoteIdent(column)
	}
	return db.quoteIdent(table) + "." + db.quoteIdent(column)
}

// quoteOrderField quotes a column name for an ORDER BY term with the
// dialect's identifier quotes, so reserved words such as "order" and "limit"
// work. A qualified name keeps its table prefix, each part quoted on its own.
func (db *database[M]) quoteOrderField(name string) string {
	if len(name) == 0 {
		return name
	}
	parts := strings.Split(name, ".")
	for i := range parts {
		if len(parts[i]) == 0 {
			continue
		}
		parts[i] = db.quoteIdent(parts[i])
	}
	return strings.Join(parts, ".")
}

// reset clears this wrapper's option fields (WithQuery, WithSelect, limits, etc.) after each
// CRUD method returns. It does not replace the underlying *gorm.DB session: GORM may still
// retain WHERE/ORDER clauses on that chain. Reusing the same Database handle for another
// independent operation is incorrect; callers must call Database[M](ctx) again for each new
// operation chain. See Database function documentation.
func (db *database[M]) reset() {
	db.mu.Lock()
	defer db.mu.Unlock()

	// reset model metadata
	var empty M
	db.m = empty
	db.typ = nil

	db.enablePurge = nil
	db.batchSize = 0
	db.noHook = false
	db.dryRun = false

	// reset sql build state
	db.buildingSQL = false
	db.sqlStatements = nil

	// reset cursor pagination
	db.cursor = types.Cursor{}

	// reset select
	db.selectColumns = nil
}

// prepare prepares the database instance for query execution by applying all configured
// query conditions, joins, and other settings to the underlying GORM database instance.
func (db *database[M]) prepare() error {
	if db.err != nil {
		return db.err
	}
	if db.ins == nil || db.ins == new(gorm.DB) {
		return ErrInvalidDB
	}
	db.typ = reflect.TypeOf(*new(M)).Elem()
	db.m = reflect.New(db.typ).Interface().(M) //nolint:errcheck

	// Set enablePurge based on model's Purge() method if not explicitly set by WithPurge().
	// Priority: WithPurge() > model.Purge() > default (soft delete)
	// - If WithPurge() was called, use the explicitly set value (highest priority)
	// - Otherwise, use model.Purge() to determine the default delete behavior
	// - model.Purge() returns true: hard delete (permanent deletion)
	// - model.Purge() returns false: soft delete (only update deleted_at field)
	if db.enablePurge == nil {
		db.enablePurge = new(db.m.Purge())
	}

	return nil
}

// Database creates and returns a generic database manipulator implementing types.Database interface.
// Provides comprehensive CRUD capabilities with advanced features like hooks and query building.
// Automatically enables debug mode when log level is set to debug.
// Required tables must exist before executing operations with the returned manipulator.
//
// Type Parameters:
//   - M: Model type that implements types.Model interface
//
// Parameters:
//   - ctx: Required context for cancellation, tracing, and request metadata.
//     In service layer operations, pass the ServiceContext directly.
//     For non-service layer operations, pass nil.
//
// Returns a database manipulator with full CRUD and query capabilities.
//
// Features:
//   - Generic type safety for model operations
//   - Automatic debug mode based on configuration
//   - Context-aware operations for tracing
//   - Default query limit protection
//   - Panic protection for uninitialized database
//   - Transaction inheritance when ctx was produced by database.Transaction or a model-hook write
//
// Transaction propagation:
//
//	Database[M](ctx) checks whether ctx carries an internal GORM transaction.
//	When present, the returned operation chain uses that transaction instead of
//	the package-level DB. This is how model hooks remain atomic without changing
//	their public signature: Create/Update/Delete create a transaction, place it in
//	the hook context, and hook code keeps calling Database[*OtherModel](ctx).
//
//	This inheritance is strictly context-scoped. Passing context.Background()
//	or any unrelated context starts a normal non-transactional operation chain.
//
// Required usage:
//
//	You must call Database[M](ctx) again for each separate operation chain. Assigning the return
//	value to a variable and running another independent operation on it afterward (e.g.
//	WithQuery(...).List(...) then Get(...) or Update(...) on the same variable) is incorrect:
//	after each method, reset() clears this wrapper's options but the underlying GORM session
//	keeps prior clauses, so later calls can combine wrong WHERE conditions, return empty models,
//	or corrupt data.
//
// Example:
//
//	var users []*User
//	// Service layer: one Database() call per operation chain (required; anything else is wrong).
//	_ = Database[*User](ctx).WithQuery(&User{Name: "John"}).List(&users)
//	u := new(User)
//	_ = Database[*User](ctx).Get(u, id)
//
//	// Non-service layer
//	_ = Database[*User](context.Background()).WithQuery(&User{Name: "John"}).List(&users)
func Database[M types.Model](ctx context.Context) types.Database[M] {
	if DB() == nil || DB() == new(gorm.DB) {
		panic("database is not initialized")
	}
	return databaseFor[M](ctx, DB())
}

// DatabaseOn is Database on an application-held database instance, typically
// built once with a dialect New function such as clickhouse.New and kept by
// the application. The chain only joins transactions opened on the same
// instance by TransactionOn. Panics on a nil instance, consistent with
// Database on an uninitialized default database.
func DatabaseOn[M types.Model](ctx context.Context, instance *gorm.DB) types.Database[M] {
	if instance == nil {
		panic("database instance cannot be nil")
	}
	return databaseFor[M](ctx, instance)
}

// databaseFor builds the operation chain for one entry-point call: it joins
// the context-carried transaction keyed by the given connection handle when
// present, and stamps the chain with that handle as its identity.
func databaseFor[M types.Model](ctx context.Context, base *gorm.DB) types.Database[M] {
	gctx := context.Background()
	if ctx != nil {
		gctx = ctx
	}

	// The handle is the chain's identity and transaction key; the running
	// connection switches to the context transaction when one exists.
	running := dbruntime.Handle(gctx, base)

	var ins *gorm.DB
	if strings.ToLower(config.App.Logger.Level) == "debug" {
		ins = running.Debug().WithContext(gctx).Limit(defaultLimit)
	} else {
		ins = running.WithContext(gctx).Limit(defaultLimit)
	}

	chain := &database[M]{
		ins:  ins,
		ctx:  gctx,
		base: base,
	}
	if isOpenTransaction(base) {
		chain.err = ErrTransactionInstance
	}
	return chain
}

// normalizeModelID runs an id through the model's own ID semantics before it
// can reach SQL, reporting false for an id the model rejects. Such an id
// cannot match any row, and answering "record not found" at the entry keeps
// the database from applying implicit string-to-integer coercion on integer
// primary keys (MySQL matches id=7 for '7abc'). Base accepts any non-empty
// string and passes through unchanged; AutoBase only accepts decimal digits.
// The probe is a clone so probeSource stays untouched.
func normalizeModelID[M types.Model](probeSource M, id string) (string, bool) {
	probe := cloneDryRunModel(probeSource)
	probe.ClearID()
	probe.SetID(id)
	id = probe.GetID()
	return id, len(id) != 0
}
