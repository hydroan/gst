package modelregistry

import (
	"reflect"
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

// TableDone reports that one queued model now has its table, and is called by
// the database runtime once table preparation returns.
func TableDone() {
	tablesPending.Add(-1)
}

// TablesPending returns how many models are queued or still having their table
// prepared.
func TablesPending() int {
	return int(tablesPending.Load())
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
