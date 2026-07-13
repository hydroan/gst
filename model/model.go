package model

import (
	"reflect"
	"strings"
	"sync"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
)

var (
	mu               sync.Mutex
	registeredModels []types.Model
)

var (
	_ types.Model = (*Base)(nil)
	_ types.Model = (*AutoBase)(nil)
	_ types.Model = (*Empty)(nil)
	_ types.Model = (*Any)(nil)
)

type (
	// Base provides common fields and default hooks for database-backed resources.
	Base = modelregistry.Base

	// AutoBase provides the same common fields and default hooks as Base but
	// uses an auto-increment integer primary key assigned by the database.
	AutoBase = modelregistry.AutoBase

	// Query enables framework-owned list query parameters when embedded by a model.
	Query = modelregistry.Query

	// Queryable marks models that opt in to framework-owned query parameters.
	Queryable = modelregistry.Queryable

	// UnsafeQuery enables unsafe list query parameters when embedded by a model.
	UnsafeQuery = modelregistry.UnsafeQuery

	// UnsafeQueryable marks models that opt in to unsafe framework query parameters.
	UnsafeQueryable = modelregistry.UnsafeQueryable

	// Pagination enables page and size query parameters when embedded by a model.
	Pagination = modelregistry.Pagination

	// Paginatable marks models that opt in to page and size query parameters.
	Paginatable = modelregistry.Paginatable

	// Cursor enables cursor query parameters when embedded by a model.
	Cursor = modelregistry.Cursor

	// Cursorable marks models that opt in to cursor query parameters.
	Cursorable = modelregistry.Cursorable

	// Empty marks a model as an action-only type that does not map to a database table.
	Empty = modelregistry.Empty

	// Any is a placeholder model for generic database operations that do not need a concrete model type.
	Any = modelregistry.Any
)

// RegisteredModels returns independent model values registered through Register.
//
// It is primarily used by tools that need to inspect registered model definitions,
// such as migration schema generation. Mutating the returned values does not
// change the registered models.
func RegisteredModels() []any {
	mu.Lock()
	defer mu.Unlock()

	models := make([]any, 0, len(registeredModels))
	for _, m := range registeredModels {
		models = append(models, newModelSnapshot(m))
	}
	return models
}

// Register registers a database-backed model for table setup and optional seed records.
//
// Models that embed Empty or Any are ignored because they do not represent
// database tables. Seed records without IDs receive generated IDs before they
// are inserted during application startup.
//
// Key features:
//   - Thread-safe concurrent registration using mutex protection
//   - Automatic ID generation for records without IDs
//
// Parameters:
//   - records: Optional initial records to be seeded into the table. Can be single or multiple records.
//
// Examples:
//
//	// Create table 'users' only
//	Register[*model.User]()
//
//	// Create table 'users' and insert one user record
//	Register[*model.User](&model.User{Name: "admin"})
//
//	// Create table 'users' and insert a single user record
//	Register[*model.User](user)
//
//	// Create table 'users' and insert multiple records
//	Register[*model.User](users...)  // where users is []*model.User
//
// NOTE:
//  1. Register is usually called from the generated model/model.go file.
//  2. Ensure the model package is imported by the application entrypoint.
//  3. The function is safe for concurrent use.
func Register[M types.Model](records ...M) {
	if !modelregistry.IsValid[M]() {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	table := reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck
	registeredModels = append(registeredModels, newModelSnapshot(table))
	modelregistry.TableChan <- table

	// NOTE: it's necessary to set id before insert.
	for i := range records {
		if len(records[i].GetID()) == 0 {
			records[i].SetID()
		}
	}

	if len(records) != 0 {
		modelregistry.RecordChan <- &modelregistry.Record{Table: table, Rows: records, Expands: table.Expands()}
	}
}

// RegisterTo registers a database-backed model on the specified database instance.
//
// Models that embed Empty or Any are ignored because they do not represent
// database tables. Unlike Register, RegisterTo preserves seed record IDs exactly
// as provided by the caller.
//
// Key features:
//   - Thread-safe concurrent registration using mutex protection
//   - Custom database instance targeting
//
// Parameters:
//   - dbname: The name of the target database instance (case-insensitive)
//   - records: Optional initial records to be seeded into the table
//
// For more details and examples, see: Register().
func RegisterTo[M types.Model](dbname string, records ...M) {
	if !modelregistry.IsValid[M]() {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	dbname = strings.ToLower(dbname)
	table := reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck

	modelregistry.TableDBChan <- &modelregistry.TableDB{Table: table, DBName: dbname}

	if len(records) != 0 {
		modelregistry.RecordChan <- &modelregistry.Record{Table: table, Rows: records, Expands: table.Expands(), DBName: dbname}
	}
}

func newModelSnapshot(m types.Model) types.Model {
	typ := reflect.TypeOf(m)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return reflect.New(typ).Interface().(types.Model) //nolint:errcheck
}
