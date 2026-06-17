package model

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gertd/go-pluralize"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"github.com/stoewer/go-strcase"
)

var pluralizeCli = pluralize.NewClient()

// GormScannerWrapper converts object to GormScanner that can be used in GORM.
// WARN: you must pass pointer to object.
func GormScannerWrapper(object any) *GormScanner {
	return &GormScanner{Object: object}
}

type GormScanner struct {
	Object any
}

func (g *GormScanner) Scan(value any) (err error) {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		err = json.Unmarshal(util.StringToBytes(v), g.Object)
	case []byte:
		err = json.Unmarshal(v, g.Object)
	default:
		err = errors.New("unsupported type, expected string or []byte")
	}
	return err
}

func (g *GormScanner) Value() (driver.Value, error) {
	data, err := json.Marshal(g.Object)
	if err != nil {
		return nil, err
	}
	return util.BytesToString(data), nil
}

func GetTableName[M types.Model]() string {
	return strcase.SnakeCase(pluralizeCli.Plural(reflect.TypeOf(*new(M)).Elem().Name()))
}

// AreTypesEqual checks if the types of M, REQ and RSP are equal
//
// If M "Empty" or "Any", return false directly.
//
// NOTE: "Empty" or "Any" will cause always use custom controller operations.
func AreTypesEqual[M types.Model, REQ types.Request, RSP types.Response]() bool {
	if IsEmpty[M]() {
		return false
	}
	typ1 := reflect.TypeFor[M]()
	typ2 := reflect.TypeFor[REQ]()
	typ3 := reflect.TypeFor[RSP]()
	return typ1 == typ2 && typ2 == typ3
}

// IsEmpty check the T is a valid struct that has at least one valid field.
// What is a valid field?
// 1. the field is not a `Empty` or pointer to `Empty`.
// 2. the field is not a `Any` or pointer to `Any`.
//
// For example, those bellow struct will returns true:
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

// IsValid check whether the T is valid model.
//
// If T is not pointer to struct, return false.
// If T has no fields, return false.
// If T fields contains `Empty` or `Any`, return false,
// otherwise return true.
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
