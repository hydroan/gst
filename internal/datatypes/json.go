package datatypes

import (
	"context"
	"database/sql/driver"

	gormdatatypes "gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormschema "gorm.io/gorm/schema"
)

// JSON is a value of type T a column stores as one JSON document: the
// settings of a record, the answer a remote call gave, a json.RawMessage for
// a document kept as it was written. A request and a response carry the
// value as T itself is written in JSON, the wrapper adding nothing. NewJSON
// wraps a value and Data returns it. An array is declared a JSONSlice and an
// object of keys known only as it is read a JSONMap, each used as the slice
// or the map it is.
//
// The column, and what is written to it and read from it, are those of the
// JSONType of gorm.io/datatypes, which holds the value.
type JSON[T any] struct {
	stored gormdatatypes.JSONType[T]
}

// NewJSON returns data as the document a column stores.
func NewJSON[T any](data T) JSON[T] {
	return JSON[T]{stored: gormdatatypes.NewJSONType(data)}
}

// Data returns the value the document holds.
func (j JSON[T]) Data() T {
	return j.stored.Data()
}

// MarshalJSON writes the value as T is written in JSON.
func (j JSON[T]) MarshalJSON() ([]byte, error) {
	return j.stored.MarshalJSON()
}

// UnmarshalJSON reads the value as T is read from JSON.
func (j *JSON[T]) UnmarshalJSON(data []byte) error {
	return j.stored.UnmarshalJSON(data)
}

// Scan reads the document a JSON column holds.
func (j *JSON[T]) Scan(value any) error {
	return j.stored.Scan(value)
}

// Value writes the document for a JSON column.
func (j JSON[T]) Value() (driver.Value, error) {
	return j.stored.Value()
}

// GormDataType declares the JSON data type.
func (j JSON[T]) GormDataType() string {
	return j.stored.GormDataType()
}

// GormDBDataType declares the JSON column of the dialect db speaks.
func (j JSON[T]) GormDBDataType(db *gorm.DB, field *gormschema.Field) string {
	return j.stored.GormDBDataType(db, field)
}

// GormValue binds the document the way the dialect db speaks takes a JSON
// value.
func (j JSON[T]) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	return j.stored.GormValue(ctx, db)
}
