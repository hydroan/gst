package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

// references:

var (
	ErrInvalidDB           = errors.New("invalid database, maybe not initialized")
	ErrUnknowDBType        = errors.New("unknow database type, only support mysql or sqlite")
	ErrNotPtrStruct        = errors.New("model is not pointer to structure")
	ErrNotPtrSlice         = errors.New("not pointer to slice")
	ErrNotPtrInt64         = errors.New("not pointer to int64")
	ErrNilCount            = errors.New("count parameter cannot be nil")
	ErrNilDest             = errors.New("dest parameter cannot be nil")
	ErrEmptyFieldName      = errors.New("field name cannot be empty")
	ErrNilValue            = errors.New("value cannot be nil")
	ErrNotAddressableModel = errors.New("model is not addressable")
	ErrNotAddressableSlice = errors.New("slice is not addressable")
	ErrNotSetSlice         = errors.New("slice cannot set")
	ErrIDRequired          = errors.New("id is required")
	ErrRecordNotFound      = gorm.ErrRecordNotFound
	ErrNilSQLBuilder       = errors.New("sql statement collector cannot be nil")
	ErrNilTransactionFunc  = errors.New("transaction function cannot be nil")
	ErrBuildSQLTransaction = errors.New("build sql does not support transaction operations")
)

// migratedModelMap records model/database pairs seen by operation builders.
// The key tracking is retained for compatibility with existing setup paths.
// Key is "dbIdentifier:modelType", value is "struct{}{}".
// dbIdentifier is the unique identifier of the database instance (e.g., pointer address of the underlying database connection).
var migratedModelMap sync.Map

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

	// options
	enablePurge *bool  // delete resource permanently, not only update deleted_at field, only works on 'Delete' method.
	enableCache bool   // using cache or not, only works 'List', 'Get', 'Count' method.
	tableName   string // support multiple custom table name, always used with the `WithDB` method.
	batchSize   int    // batch size for bulk operations. affects Create, Update, Delete.
	noHook      bool   // disable model hook.
	dryRun      bool   // build SQL without database I/O, hooks, cache mutation, or object field filling.

	// sql
	buildingSQL   bool // collect generated SQL statements for WithBuildSQL.
	sqlStatements *[]types.SQLStatement

	// cursor pagination
	cursorField  string // field used for cursor pagination, default is "id"
	cursorValue  string // cursor value for pagination
	cursorNext   bool   // direction of cursor pagination, true for next page, false for previous page
	enableCursor bool   // enable cursor pagination

	// select
	selectColumns []string

	// rollback control
	rollbackFunc func() // rollback function for manual transaction control

	shouldAutoMigrate *bool
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

func (db *database[M]) quoteOrderField(name string) string {
	if len(name) == 0 {
		return name
	}
	if strings.HasPrefix(name, "`") || strings.HasPrefix(name, "\"") || strings.HasPrefix(name, "[") {
		return name
	}
	// Preserve raw expressions like functions or JSON operators.
	if strings.ContainsAny(name, "()*+-/") {
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

func (db *database[M]) regexpOperator() string {
	if db == nil || db.ins == nil || db.ins.Dialector == nil {
		return "REGEXP"
	}
	switch strings.ToLower(db.ins.Dialector.Name()) {
	case "postgres":
		return "~"
	default:
		return "REGEXP"
	}
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
	db.enableCache = false
	db.tableName = ""
	db.batchSize = 0
	db.noHook = false
	db.shouldAutoMigrate = nil
	db.dryRun = false

	// reset sql build state
	db.buildingSQL = false
	db.sqlStatements = nil

	// reset cursor pagination fields
	db.cursorField = ""
	db.cursorValue = ""
	db.cursorNext = true
	db.enableCursor = false

	// reset select
	db.selectColumns = nil

	// reset rollback function
	db.rollbackFunc = nil
}

// prepare prepares the database instance for query execution by applying all configured
// query conditions, joins, and other settings to the underlying GORM database instance.
func (db *database[M]) prepare() error {
	if db.ins == nil || db.ins == new(gorm.DB) {
		return ErrInvalidDB
	}
	db.typ = reflect.TypeOf(*new(M)).Elem()
	db.m = reflect.New(db.typ).Interface().(M) //nolint:errcheck

	// AutoMigrate is intentionally disabled for operation chains. Framework
	// initialization and custom callers are responsible for schema creation.
	// if db.shouldAutoMigrate != nil && *db.shouldAutoMigrate {
	// 	session := db.ins
	// 	if tableName := db.m.GetTableName(); len(tableName) > 0 {
	// 		session = session.Table(tableName)
	// 	}
	// 	if err := session.AutoMigrate(db.m); err != nil {
	// 		return err
	// 	}
	// }

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
// Provides comprehensive CRUD capabilities with advanced features like caching, hooks, and query building.
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
	gctx := context.Background()
	if ctx != nil {
		gctx = ctx
	}

	var ins *gorm.DB
	if strings.ToLower(config.App.Logger.Level) == "debug" {
		ins = DB().Debug().WithContext(gctx).Limit(defaultLimit)
	} else {
		ins = DB().WithContext(gctx).Limit(defaultLimit)
	}

	db := &database[M]{
		ins: ins,
		ctx: gctx,
	}

	// Track database identifier + model type for compatibility with existing setup bookkeeping.
	dbIdentifier := getDBIdentifier(DB())
	modelType := reflect.TypeFor[M]().String()
	migrationKey := fmt.Sprintf("%s:%s", dbIdentifier, modelType)
	if _, loaded := migratedModelMap.LoadOrStore(migrationKey, struct{}{}); !loaded {
		flag := new(bool)
		*flag = true
		db.shouldAutoMigrate = flag
	}

	return db
}
