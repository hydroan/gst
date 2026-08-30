package modelregistry

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/hydroan/gst/types"
)

// TableChan is the internal queue for default-database table registration.
// It receives model values from enqueueTable for processing by dbruntime.InitDatabase.
var TableChan = make(chan types.Model, 10240)

// tablesPending counts the models queued for table registration whose table is
// not ready yet. The queue length alone cannot answer that: a receive takes a
// model off the channel long before its table exists, so a caller waiting on
// len(TableChan) is told the work is done while a CREATE TABLE is still
// running. Counting from the moment a model is queued leaves no such window.
var tablesPending atomic.Int32

// enqueueTable queues a model for table registration. It is the only way to
// put a model on TableChan, so that the pending count can never miss one, and
// it is reached through RegisterTable alone: a model that took the queue
// without being recorded would have a table nothing knows it declared, which
// SchemaFingerprint would then answer for a schema it never saw.
func enqueueTable(m types.Model) {
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

	enqueueTable(table)
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
