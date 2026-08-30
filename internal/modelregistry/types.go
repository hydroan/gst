package modelregistry

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/hydroan/gst/types"
)

// TableChan is the internal queue for default-database table registration.
// It receives model values from EnqueueTable for processing by dbruntime.InitDatabase.
var TableChan = make(chan types.Model, 10240)

// tablesPending counts the models queued for table registration whose table is
// not ready yet. The queue length alone cannot answer that: a receive takes a
// model off the channel long before its table exists, so a caller waiting on
// len(TableChan) is told the work is done while a CREATE TABLE is still
// running. Counting from the moment a model is queued leaves no such window.
var tablesPending atomic.Int32

// EnqueueTable queues a model for table registration. It is the only way to
// put a model on TableChan, so that the pending count can never miss one.
func EnqueueTable(m types.Model) {
	tablesPending.Add(1)
	TableChan <- m
}

// tablesChanged carries a wakeup for a waiter whenever a table finishes
// preparing, so that waiting for the queue to drain costs no polling latency.
// It is buffered and written without blocking on purpose: the signal carries
// no information beyond "read the count again", so a wakeup already queued
// stands in for the one being sent.
var tablesChanged = make(chan struct{}, 1)

// TableDone reports that one queued model now has its table, and is called by
// the database runtime once table preparation returns.
func TableDone() {
	tablesPending.Add(-1)
	select {
	case tablesChanged <- struct{}{}:
	default:
	}
}

// TablesChanged returns the channel a waiter blocks on until a table finishes
// preparing. A receive means the pending count is worth reading again, never
// that it reached zero: TablesPending stays the only authority on that, which
// is what keeps a stale or coalesced wakeup harmless.
func TablesChanged() <-chan struct{} {
	return tablesChanged
}

// TablesPending returns how many models are queued or still having their table
// prepared.
func TablesPending() int {
	return int(tablesPending.Load())
}

// registeredModels holds one value per model registered for table setup, in
// registration order. The queue cannot stand in for it: the database runtime
// drains the queue, so everything that needs the registered set afterwards —
// a schema dump, a migration plan — has to read it from here.
var (
	registeredMu     sync.Mutex
	registeredModels []types.Model
)

// RegisterTable records M as a registered model and queues it for table
// setup. Models that embed Empty are ignored: they map to no table.
//
// It is the single entry point for both steps, so the recorded set and the
// queue can never disagree about what was registered.
func RegisterTable[M types.Model]() {
	if !IsValid[M]() {
		return
	}
	table := newModelSnapshot[M]()

	registeredMu.Lock()
	registeredModels = append(registeredModels, newModelSnapshot[M]())
	registeredMu.Unlock()

	EnqueueTable(table)
}

// RegisteredModels returns independent values of the models registered
// through RegisterTable. Mutating them does not change what is registered.
func RegisteredModels() []any {
	registeredMu.Lock()
	defer registeredMu.Unlock()

	models := make([]any, 0, len(registeredModels))
	for _, m := range registeredModels {
		models = append(models, newModelSnapshotOf(m))
	}
	return models
}

// newModelSnapshot returns a fresh zero value of M.
func newModelSnapshot[M types.Model]() M {
	return reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck
}

// newModelSnapshotOf returns a fresh zero value of m's concrete type.
func newModelSnapshotOf(m types.Model) types.Model {
	typ := reflect.TypeOf(m)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return reflect.New(typ).Interface().(types.Model) //nolint:errcheck
}

// AreTypesEqual reports whether M, REQ, and RSP are the same concrete type.
//
// Empty models always return false so custom controller operations are used.
func AreTypesEqual[M types.Model, REQ types.Request, RSP types.Response]() bool {
	if IsEmpty[M]() {
		return false
	}
	typ1 := reflect.TypeFor[M]()
	typ2 := reflect.TypeFor[REQ]()
	typ3 := reflect.TypeFor[RSP]()
	return typ1 == typ2 && typ2 == typ3
}

// IsEmpty reports whether T has no fields beyond Empty markers.
//
// For example, these structs return true:
//
//	type Login struct {
//		model.Empty
//	}
//
//	type Login struct {
//		*model.Empty
//	}
//
//	type Logout struct{
//	}
func IsEmpty[T any]() bool {
	typ := reflect.TypeFor[T]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return true
	}
	if typ.NumField() == 0 {
		return true
	}

	invalidFieldCount := 0
	for field := range typ.Fields() {
		ftyp := field.Type
		for ftyp.Kind() == reflect.Pointer {
			ftyp = ftyp.Elem()
		}
		if ftyp == reflect.TypeFor[Empty]() {
			invalidFieldCount++
		}
	}

	return typ.NumField() == invalidFieldCount
}

// IsValid reports whether T is a database-backed model.
//
// T must be a pointer to a non-empty struct and must not embed Empty.
func IsValid[T any]() bool {
	typ := reflect.TypeFor[T]()

	// T type not pointer, return false.
	if typ.Kind() != reflect.Pointer {
		return false
	}

	// T type not struct, return false
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		return false
	}

	// T has no fields, return false
	if typ.NumField() == 0 {
		return false
	}

	// T fields contains `Empty`, return false
	for field := range typ.Fields() {
		if field.Type == reflect.TypeFor[Empty]() {
			return false
		}
	}

	return true
}
