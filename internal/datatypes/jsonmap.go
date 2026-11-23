package datatypes

import (
	"context"
	"database/sql/driver"

	gormdatatypes "gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormschema "gorm.io/gorm/schema"
)

// JSONMap is a map a column stores as one JSON object, its keys known only
// as it is read: the metadata of a record, a payload kept as it came. It is
// the map it is declared over, read and written by key as one, and written
// in a request and a response as the object a map[string]any is.
//
// The column, what is written to it and read from it, and the JSON of the
// map are those of the JSONMap of gorm.io/datatypes, which the value is
// stored through by conversion.
type JSONMap map[string]any //nolint:recvcheck // UnmarshalJSON and Scan write through the pointer, MarshalJSON and Value read the value: the receiver pairs encoding/json and database/sql document.

// MarshalJSON writes the map as a JSON object, and a nil map as null.
func (m JSONMap) MarshalJSON() ([]byte, error) {
	return gormdatatypes.JSONMap(m).MarshalJSON()
}

// UnmarshalJSON reads the map from a JSON object.
func (m *JSONMap) UnmarshalJSON(data []byte) error {
	return (*gormdatatypes.JSONMap)(m).UnmarshalJSON(data)
}

// Scan reads the object a JSON column holds.
func (m *JSONMap) Scan(value any) error {
	return (*gormdatatypes.JSONMap)(m).Scan(value)
}

// Value writes the object for a JSON column.
func (m JSONMap) Value() (driver.Value, error) {
	return gormdatatypes.JSONMap(m).Value()
}

// GormDataType declares the JSON data type.
func (m JSONMap) GormDataType() string {
	return gormdatatypes.JSONMap(m).GormDataType()
}

// GormDBDataType declares the JSON column of the dialect db speaks.
func (m JSONMap) GormDBDataType(db *gorm.DB, field *gormschema.Field) string {
	return gormdatatypes.JSONMap(m).GormDBDataType(db, field)
}

// GormValue binds the object the way the dialect db speaks takes a JSON
// value.
func (m JSONMap) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	return gormdatatypes.JSONMap(m).GormValue(ctx, db)
}
