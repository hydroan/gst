package database

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// sqlite loses the declared type of an expression column: an aggregated
// projection such as MAX(closed_at) reaches the driver without a decltype,
// so the driver hands back the stored TEXT instead of the parsed time.Time
// it returns for a plain column read, and database/sql cannot scan that
// string into a time field. On sqlite alone, the terminal scan therefore
// stands a mirror struct in for the caller's result type: every time-shaped
// field is swapped for sqliteTimeValue, which parses the driver's own
// timestamp formats, and the scanned rows are copied back field by field.

// sqliteDialectName is what the gorm sqlite dialector calls itself.
const sqliteDialectName = "sqlite"

var (
	timeType     = reflect.TypeFor[time.Time]()
	timePtrType  = reflect.TypeFor[*time.Time]()
	nullTimeType = reflect.TypeFor[sql.NullTime]()

	sqliteTimeValueType = reflect.TypeFor[sqliteTimeValue]()
)

// scanRowsInto runs the terminal scan of tx into dest, through the time
// mirror when the dialect needs one.
func scanRowsInto[R any](tx *gorm.DB, dest *[]R) error {
	mirrorType, ok := scanMirrorType[R](tx)
	if !ok {
		return tx.Scan(dest).Error
	}

	rows := reflect.New(reflect.SliceOf(mirrorType))
	if err := tx.Scan(rows.Interface()).Error; err != nil {
		return err
	}
	mirrored := rows.Elem()
	for i := range mirrored.Len() {
		var row R
		copyMirrorRow(mirrored.Index(i), reflect.ValueOf(&row).Elem())
		*dest = append(*dest, row)
	}
	return nil
}

// scanRowInto is the one-row variant of scanRowsInto.
func scanRowInto[R any](tx *gorm.DB, dest *R) error {
	mirrorType, ok := scanMirrorType[R](tx)
	if !ok {
		return tx.Scan(dest).Error
	}

	row := reflect.New(mirrorType)
	if err := tx.Scan(row.Interface()).Error; err != nil {
		return err
	}
	copyMirrorRow(row.Elem(), reflect.ValueOf(dest).Elem())
	return nil
}

// scanMirrorType returns the stand-in type for R when the scan needs one,
// which is only the case on sqlite: every other dialect delivers time values
// already parsed.
func scanMirrorType[R any](tx *gorm.DB) (reflect.Type, bool) {
	if tx.Dialector.Name() != sqliteDialectName {
		return nil, false
	}
	return sqliteTimeMirrorType(reflect.TypeFor[R]())
}

// sqliteTimeMirrorType returns the scan-side stand-in for a result type: the
// same struct with every time-shaped field replaced by sqliteTimeValue. The
// second return is false when no stand-in is needed or possible — the type
// has no time-shaped fields, is no struct, or carries unexported fields,
// which reflect.StructOf cannot rebuild; those types scan the regular way.
// Embedded structs are not descended into.
func sqliteTimeMirrorType(rt reflect.Type) (reflect.Type, bool) {
	if rt.Kind() != reflect.Struct {
		return nil, false
	}

	fields := make([]reflect.StructField, rt.NumField())
	mirrored := false
	for i := range rt.NumField() {
		field := rt.Field(i)
		if len(field.PkgPath) > 0 {
			return nil, false
		}
		switch field.Type {
		case timeType, timePtrType, nullTimeType:
			field.Type = sqliteTimeValueType
			mirrored = true
		}
		fields[i] = field
	}
	if !mirrored {
		return nil, false
	}
	return reflect.StructOf(fields), true
}

// copyMirrorRow writes one scanned mirror row into the caller's row,
// converting the stand-in fields back to the shape the caller declared.
func copyMirrorRow(mirror, dest reflect.Value) {
	for i := range dest.NumField() {
		source := mirror.Field(i)
		if source.Type() == sqliteTimeValueType {
			value, _ := source.Interface().(sqliteTimeValue)
			value.assignTo(dest.Field(i))
			continue
		}
		dest.Field(i).Set(source)
	}
}

// sqliteTimeValue is the scan target standing in for a time-shaped result
// field. Absence stays distinguishable through valid, so the value converts
// back to any of the shapes it replaces.
type sqliteTimeValue struct {
	t     time.Time
	valid bool
}

// Value implements driver.Valuer, which is what makes gorm read the field as
// data rather than trying to resolve it as a relation. The mirror is only
// ever a scan target, never a bind parameter, so absence needs no nil here
// and the zero time stands in for it.
func (v *sqliteTimeValue) Value() (driver.Value, error) {
	return v.t, nil
}

// Scan implements sql.Scanner. Real time values pass through, TEXT is parsed
// the way the driver itself would have.
func (v *sqliteTimeValue) Scan(src any) error {
	switch s := src.(type) {
	case nil:
		*v = sqliteTimeValue{}
		return nil
	case time.Time:
		*v = sqliteTimeValue{t: s, valid: true}
		return nil
	case string:
		return v.parse(s)
	case []byte:
		return v.parse(string(s))
	default:
		return errors.Newf("cannot scan %T into a time field", src)
	}
}

// sqliteTimestampFormats mirrors the mattn/go-sqlite3 SQLiteTimestampFormats
// list, which is the authority on what timestamp shapes the driver writes and
// accepts. It is copied rather than imported: that symbol lives in the
// driver's cgo implementation, and a CGO_ENABLED=0 build — the usual shape of
// a static deployment binary — compiles the driver's stub instead, where the
// symbol does not exist.
var sqliteTimestampFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// parse reads s with the driver's own format list, in UTC like the driver,
// so whatever timestamp shape the driver stored parses back unchanged.
func (v *sqliteTimeValue) parse(s string) error {
	trimmed := strings.TrimSuffix(s, "Z")
	for _, format := range sqliteTimestampFormats {
		if t, err := time.ParseInLocation(format, trimmed, time.UTC); err == nil {
			*v = sqliteTimeValue{t: t, valid: true}
			return nil
		}
	}
	return errors.Newf("cannot parse %q as a time value", s)
}

// assignTo writes the scanned value into the field the mirror stood in for.
func (v *sqliteTimeValue) assignTo(dest reflect.Value) {
	switch dest.Type() {
	case timeType:
		dest.Set(reflect.ValueOf(v.t))
	case timePtrType:
		if !v.valid {
			dest.SetZero()
			return
		}
		t := v.t
		dest.Set(reflect.ValueOf(&t))
	case nullTimeType:
		dest.Set(reflect.ValueOf(sql.NullTime{Time: v.t, Valid: v.valid}))
	}
}
