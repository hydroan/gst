package modelregistry

import (
	"reflect"

	"github.com/gertd/go-pluralize"
	"github.com/hydroan/gst/types"
	"github.com/stoewer/go-strcase"
)

var pluralizeCli = pluralize.NewClient()

// GetTableName returns the default table name for a model type.
func GetTableName[M types.Model]() string {
	return strcase.SnakeCase(pluralizeCli.Plural(reflect.TypeOf(*new(M)).Elem().Name()))
}

// AreTypesEqual reports whether M, REQ, and RSP are the same concrete type.
//
// Empty and Any models always return false so custom controller operations are used.
func AreTypesEqual[M types.Model, REQ types.Request, RSP types.Response]() bool {
	if IsEmpty[M]() {
		return false
	}
	typ1 := reflect.TypeFor[M]()
	typ2 := reflect.TypeFor[REQ]()
	typ3 := reflect.TypeFor[RSP]()
	return typ1 == typ2 && typ2 == typ3
}

// IsEmpty reports whether T has no fields beyond Empty or Any markers.
//
// For example, these structs return true:
//
//	type Login struct {
//		model.Empty
//	}
//
//	type Login struct {
//		model.Empty
//		model.Any
//	}
//
//	type Login struct {
//		*model.Empty
//		model.Any
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
		if ftyp == reflect.TypeFor[Empty]() || ftyp == reflect.TypeFor[Any]() {
			invalidFieldCount++
		}
	}

	return typ.NumField() == invalidFieldCount
}

// IsValid reports whether T is a database-backed model.
//
// T must be a pointer to a non-empty struct and must not embed Empty or Any.
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

	// T fields contains `Empty` or `Any`, return false
	for field := range typ.Fields() {
		if field.Type == reflect.TypeFor[Empty]() || field.Type == reflect.TypeFor[Any]() {
			return false
		}
	}

	return true
}
