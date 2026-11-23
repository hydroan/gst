package datatypes

import (
	"context"
	"database/sql/driver"

	gormdatatypes "gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormschema "gorm.io/gorm/schema"
)

// JSONSlice is a slice of T a column stores as one JSON array: the tags of a
// record, the steps of a rule. It is the slice it is declared over, indexed,
// ranged over and appended to as one, and written in a request and a
// response as the array a []T is; NewJSONSlice converts a []T without
// naming T.
//
// The column, and what is written to it and read from it, are those of the
// JSONSlice of gorm.io/datatypes, which the value is stored through by
// conversion.
type JSONSlice[T any] []T

// NewJSONSlice returns s as the array a column stores.
func NewJSONSlice[T any](s []T) JSONSlice[T] {
	return JSONSlice[T](s)
}

// Scan reads the array a JSON column holds.
func (s *JSONSlice[T]) Scan(value any) error {
	return (*gormdatatypes.JSONSlice[T])(s).Scan(value)
}

// Value writes the array for a JSON column.
func (s JSONSlice[T]) Value() (driver.Value, error) {
	return gormdatatypes.JSONSlice[T](s).Value()
}

// GormDataType declares the JSON data type.
func (s JSONSlice[T]) GormDataType() string {
	return gormdatatypes.JSONSlice[T](s).GormDataType()
}

// GormDBDataType declares the JSON column of the dialect db speaks.
func (s JSONSlice[T]) GormDBDataType(db *gorm.DB, field *gormschema.Field) string {
	return gormdatatypes.JSONSlice[T](s).GormDBDataType(db, field)
}

// GormValue binds the array the way the dialect db speaks takes a JSON
// value.
func (s JSONSlice[T]) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	return gormdatatypes.JSONSlice[T](s).GormValue(ctx, db)
}
