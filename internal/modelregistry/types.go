package modelregistry

import (
	"reflect"

	"github.com/hydroan/gst/internal/types"
)

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

// IsEmpty reports whether T carries no data of its own, so there is nothing
// to bind a request into or describe as a body: T, its pointers removed, is
// not a struct, is a struct without fields, or is a struct whose every field
// is an Empty marker. The controllers ask it to hand an action model to its
// service instead of the default CRUD flow (see AreTypesEqual), and the
// OpenAPI generator asks it to leave a request or response body out.
//
// For example, it reports true for Login and Logout, and false for Signup,
// which carries a field beside the marker:
//
//	type Login struct {
//		model.Empty
//	}
//
//	type Logout struct{}
//
//	type Signup struct {
//		Email string
//		model.Empty
//	}
//
// Models embed Empty by value; RegisterTable and gg gen reject *model.Empty.
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
		if isEmptyMarker(field.Type) {
			invalidFieldCount++
		}
	}

	return typ.NumField() == invalidFieldCount
}

// IsValid reports whether T is a database-backed model, the kind
// RegisterTable queues for table setup: T is a pointer to a struct that has
// fields, none of them an Empty marker. For example, with
//
//	type Record struct {
//		Name string
//		model.Base
//	}
//
//	type Login struct {
//		Name string
//		model.Empty
//	}
//
// IsValid reports true for *Record, and false for Record, which is not a
// pointer, for *Login, a virtual model, and for a pointer to a struct
// without fields.
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
		if isEmptyMarker(field.Type) {
			return false
		}
	}

	return true
}

// isEmptyMarker reports whether a field of type typ is an Empty marker: Empty
// itself, or a pointer to it.
func isEmptyMarker(typ reflect.Type) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ == reflect.TypeFor[Empty]()
}
