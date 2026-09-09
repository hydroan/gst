package database

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strconv"
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
	valuerType          = reflect.TypeFor[driver.Valuer]()
)

// scanRowsInto runs the terminal scan of tx into dest, through the time
// mirror when the dialect needs one.
func scanRowsInto[R any](tx *gorm.DB, dest *[]R) error {
	mirrorType, ok := scanMirrorType[R](tx)
	if !ok {
		// First-hand exit of a stack-less GORM/driver error; see the
		// error-stack contract in doc.go. WithStack passes nil through.
		return errors.WithStack(tx.Scan(dest).Error)
	}

	rows := reflect.New(reflect.SliceOf(mirrorType))
	if err := tx.Scan(rows.Interface()).Error; err != nil {
		return errors.WithStack(err)
	}
	mirrored := rows.Elem()
	for i := range mirrored.Len() {
		var row R
		copyMirrorRow(mirrored.Index(i), rowStruct(reflect.ValueOf(&row).Elem()))
		*dest = append(*dest, row)
	}
	return nil
}

// scanRowInto is the one-row variant of scanRowsInto.
func scanRowInto[R any](tx *gorm.DB, dest *R) error {
	mirrorType, ok := scanMirrorType[R](tx)
	if !ok {
		return errors.WithStack(tx.Scan(dest).Error)
	}

	row := reflect.New(mirrorType)
	if err := tx.Scan(row.Interface()).Error; err != nil {
		return errors.WithStack(err)
	}
	copyMirrorRow(row.Elem(), rowStruct(reflect.ValueOf(dest).Elem()))
	return nil
}

// scanMirrorType returns the stand-in type for R when the scan needs one,
// which is only the case on sqlite: every other dialect delivers time values
// already parsed. A pointer row type is mirrored through its struct, and an
// embedded struct through its fields, the shapes the result row is validated
// through.
func scanMirrorType[R any](tx *gorm.DB) (reflect.Type, bool) {
	if tx.Dialector.Name() != sqliteDialectName {
		return nil, false
	}
	typ := reflect.TypeFor[R]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return sqliteTimeMirrorType(typ)
}

// rowStruct returns the struct a row value holds, allocating through a
// pointer row type, so a mirror row copies into R whether R is the struct or
// a pointer to it.
func rowStruct(row reflect.Value) reflect.Value {
	for row.Kind() == reflect.Pointer {
		if row.IsNil() {
			row.Set(reflect.New(row.Type().Elem()))
		}
		row = row.Elem()
	}
	return row
}

// sqliteTimeMirrorType returns the scan-side stand-in for a result type: the
// same struct with every time-shaped field replaced by sqliteTimeValue, an
// embedded struct, anonymous or named and tagged embedded, replaced by its
// own stand-in when it carries one. An anonymous struct becomes a named
// field gorm flattens through the embedded tag: reflect.StructOf embeds a
// type carrying methods in the first field alone, and a non-pointer one
// only as the sole field, and the caller's type may well carry some.
// Unexported fields are left out of the stand-in,
// which reflect.StructOf cannot build with, and copied back around; gorm
// reads none of them. The second return is false when no stand-in is needed
// or possible — the type has no time-shaped fields, is no struct, or embeds
// a non-struct type carrying methods or shaped like a pointer, which
// reflect.StructOf cannot embed; those types scan the regular way.
func sqliteTimeMirrorType(rt reflect.Type) (reflect.Type, bool) {
	if rt.Kind() != reflect.Struct {
		return nil, false
	}

	fields := make([]reflect.StructField, 0, rt.NumField())
	mirrored := false
	for field := range rt.Fields() {
		if len(field.PkgPath) > 0 {
			continue
		}
		switch field.Type {
		case timeType, timePtrType, nullTimeType:
			field.Type = sqliteTimeValueType
			mirrored = true
		default:
			mirroredField, ok := mirrorEmbeddedField(field)
			if !ok {
				return nil, false
			}
			if mirroredField.Type != field.Type {
				mirrored = true
			}
			field = mirroredField
		}
		fields = append(fields, field)
	}
	if !mirrored {
		return nil, false
	}
	return reflect.StructOf(fields), true
}

// mirrorEmbeddedField returns the stand-in for a field that may embed a
// struct: an anonymous struct, or a named one gorm flattens through the
// embedded tag, descended into for its time-shaped fields and, when
// anonymous, named so that reflect.StructOf never embeds a type carrying
// methods; the embedded tag is added to the settings the field carries. A
// struct gorm reads as one column, one implementing driver.Valuer, is left
// to its own Scan under its name; an anonymous field of any other kind
// cannot be rebuilt and reports false.
func mirrorEmbeddedField(field reflect.StructField) (reflect.StructField, bool) {
	if !field.Anonymous && !hasGormSetting(field.Tag, "EMBEDDED") {
		return field, true
	}
	typ := field.Type
	pointer := false
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
		pointer = true
	}
	if typ.Kind() != reflect.Struct {
		// A non-struct embeds as it is when reflect.StructOf can embed it:
		// carrying no methods and not shaped like a pointer.
		return field, !field.Anonymous || (typ.NumMethod() == 0 && !pointerShaped(field.Type))
	}
	if typ.Implements(valuerType) || reflect.PointerTo(typ).Implements(valuerType) {
		// One column, read by name; embedding it would flatten it.
		field.Anonymous = false
		return field, true
	}
	if inner, ok := sqliteTimeMirrorType(typ); ok {
		if pointer {
			inner = reflect.PointerTo(inner)
		}
		field.Type = inner
	}
	if field.Anonymous {
		field.Anonymous = false
		if !hasGormSetting(field.Tag, "EMBEDDED") {
			field.Tag = withGormSetting(field.Tag, "embedded")
		}
	}
	return field, true
}

// hasGormSetting reports whether a gorm tag carries the setting, named as
// gorm names it.
func hasGormSetting(tag reflect.StructTag, name string) bool {
	for setting := range strings.SplitSeq(tag.Get("gorm"), ";") {
		key, _, _ := strings.Cut(setting, ":")
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return true
		}
	}
	return false
}

// withGormSetting prepends a setting to the field's gorm tag, keeping the
// settings it already carries and the tags of other keys as they are.
func withGormSetting(tag reflect.StructTag, setting string) reflect.StructTag {
	value := setting
	existing, found := tag.Lookup("gorm")
	if len(existing) > 0 {
		value += ";" + existing
	}
	quoted := "gorm:" + strconv.Quote(value)
	if !found {
		if len(tag) == 0 {
			return reflect.StructTag(quoted)
		}
		return reflect.StructTag(quoted + " " + string(tag))
	}
	start, end := tagKeySpan(string(tag), "gorm")
	return reflect.StructTag(string(tag)[:start] + quoted + string(tag)[end:])
}

// tagKeySpan finds the key:"value" pair of a key in a struct tag, scanning
// the way reflect.StructTag.Lookup does, and returns its bounds; the key is
// known to be present.
func tagKeySpan(tag, key string) (int, int) {
	i := 0
	for i < len(tag) {
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		start := i
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		name := tag[start:i]
		if i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		i += 2
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		i++
		if name == key {
			return start, i
		}
	}
	return len(tag), len(tag)
}

// pointerShaped reports whether a type is represented by a pointer, which
// reflect.StructOf cannot embed beside another field.
func pointerShaped(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

// copyMirrorRow writes one scanned mirror row into the caller's row,
// converting the stand-in fields back to the shape the caller declared, an
// embedded stand-in field by field.
func copyMirrorRow(mirror, dest reflect.Value) {
	for i := range dest.NumField() {
		field := dest.Type().Field(i)
		if len(field.PkgPath) > 0 {
			continue
		}
		source := mirror.FieldByName(field.Name)
		switch {
		case source.Type() == sqliteTimeValueType:
			value, _ := reflect.TypeAssert[sqliteTimeValue](source)
			value.assignTo(dest.Field(i))
		case source.Type() != dest.Field(i).Type():
			if source.Kind() == reflect.Pointer {
				if source.IsNil() {
					dest.Field(i).SetZero()
					continue
				}
				target := reflect.New(dest.Field(i).Type().Elem())
				copyMirrorRow(source.Elem(), target.Elem())
				dest.Field(i).Set(target)
				continue
			}
			copyMirrorRow(source, dest.Field(i))
		default:
			dest.Field(i).Set(source)
		}
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
