package modelregistry

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"gorm.io/gorm/schema"
)

// Version is the optimistic-locking column type. A model opts in by declaring
// a NAMED top-level field of this type, with exactly this tag shape:
//
//	type Config struct {
//		Name    string        `json:"name"`
//		Version model.Version `json:"version,omitempty" gorm:"not null;default:1"`
//
//		model.Base
//	}
//
// The json tag must keep the field serializable (not "-": clients read the
// version they must hand back) and must carry omitempty: an unset version on
// a struct a client marshals would otherwise serialize as an explicit
// "version":0, which the write paths rightly reject — omitempty keeps
// "unset" out of request bodies entirely. Stored versions start at 1, so
// omitempty never hides a real version on the way out.
//
// The shape is enforced, not advised: "gg gen" fills the gorm tag in for a
// bare declaration, "gg check" reports deviations, and the framework panics
// on first touch of a model that embeds Version or carries a wrong tag — the
// two defects would otherwise be silent (an embedded Version is simply not
// recognized, and a wrong default locks adopted rows out of Update).
//
// The default:1 is load-bearing, not decoration. Adopting the lock on a table
// that already holds rows adds the column through a migration, and the
// database backfills existing rows with the column default. With default:1
// every existing row becomes version 1 — "this row exists" is exactly what
// version 1 means — and stays updatable. Without it the backfill is the type
// zero, and every existing row is locked out of Update forever: a zero
// version fails ErrVersionRequired, and only Update could have raised it.
// The framework never relies on the default when writing (Create stamps 1
// itself), so the "zero value swallowed by a column default" insert hazard
// cannot trigger.
//
// The declaration is the whole opt-in: models without the field pay nothing,
// and every write path of the framework database layer changes behavior for
// models that carry it. The contract, per operation:
//
//	Create      no check; a zero version is initialized to 1 (a non-zero
//	            value is kept, for imports that carry history).
//	Update      the object MUST carry its version: the statement matches
//	            WHERE version = <carried>, a zero version fails with
//	            ErrVersionRequired, and zero matched rows fail with
//	            ErrStaleObject. On success the version is bumped by one, in
//	            the row and in the object.
//	UpdateByID  exempt from the check (the caller holds no object, so there
//	            is no version to compare), but the statement still bumps the
//	            column so everyone else's carried version expires. An
//	            explicit version assignment takes the bump over — beware
//	            that assigning a LOWER value than the row's current version
//	            revives stale writes: a writer still holding that older
//	            version would suddenly match again. Reset forward, never
//	            backward.
//	Delete      checked when the object carries a non-zero version — a
//	            delete decided over stale data must fail like a stale update
//	            — and unconditional when it does not: deleting by a bare id
//	            is the deliberate way around the lock. Soft delete never
//	            bumps: a soft-deleted row is unreachable to every later
//	            write already, which is a stronger expiry than any bump.
//	Upsert      exempt from the check (merge-overwrite semantics); the
//	            conflict-update branch bumps the row's own version instead
//	            of writing the object's, so an upsert can never move a row's
//	            version backwards.
//
// Why "carry the version" matters: the version is read together with the row
// and handed back on the write, so a write only succeeds against the exact
// row state the caller saw. Two writers load version 3; the first save moves
// the row to 4; the second save matches nothing and fails with ErrStaleObject
// instead of silently overwriting the first. The same holds for a delete
// decided over a stale screen.
//
// It follows that a version is always carried FROM a read — a Get, a List, a
// value the client sent back with the form it loaded. Making one up, or
// reading the current version right before the write just to satisfy the
// check, is not a workaround but a disarmed lock at full price: the write
// then matches whatever the row holds, which is exactly the overwrite the
// lock exists to refuse. Code that genuinely wants an unchecked write of
// specific columns has UpdateByID.
//
// The zero value means "no version carried". It is deliberately not a valid
// stored version — stored versions start at 1 — so a zero always identifies
// an object that was never read from the database.
//
// The column type is a plain int64 (BIGINT). ClickHouse instances are outside
// this contract: the dialect has no matched-rows update semantics for the
// check to build on.
type Version int64

// versionField describes where a model keeps its Version column. The zero
// value (has == false) is the answer for models without one.
type versionField struct {
	index  int    // top-level struct field index of the Version field
	column string // database column name the field maps to
	has    bool
}

// versionFieldCache memoizes the per-type detection; a type's fields are
// fixed for the life of the binary.
var versionFieldCache sync.Map

// versionFieldOf resolves the Version field of m's type. Only top-level
// fields count: the field is a deliberate per-model declaration, not
// something to inherit through an embedded struct, and a nested match would
// make "which struct owns the lock" ambiguous.
func versionFieldOf(m any) versionField {
	typ := reflect.TypeOf(m)
	if typ == nil {
		return versionField{}
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return versionField{}
	}

	if cached, ok := versionFieldCache.Load(typ); ok {
		return cached.(versionField) //nolint:errcheck
	}

	info := versionField{}
	versionType := reflect.TypeFor[Version]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Type != versionType {
			continue
		}
		// The declaration shape is enforced here, at the one point every
		// code path funnels through, because both defects are silent
		// otherwise: an embedded Version is not recognized and the lock
		// quietly never engages, and a missing default:1 backfills adopted
		// rows to zero and locks them out of Update forever. A panic on
		// first touch (model registration, or the first write) turns both
		// into an immediate, explained startup failure.
		if field.Anonymous {
			panic(fmt.Sprintf(
				"model %s embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"not null;default:1\"` (an embedded Version is not recognized and the lock would silently not engage)",
				typ))
		}
		if missing := VersionTagMissing(field.Tag); len(missing) > 0 {
			quoted := make([]string, len(missing))
			for i, setting := range missing {
				quoted[i] = "`" + setting + "`"
			}
			panic(fmt.Sprintf(
				"model %s field %s (model.Version) must carry json:\",omitempty\" and gorm:\"not null;default:1\", missing %s; default:1 backfills existing rows to a live version when the column is added — without it they are locked out of Update forever — and omitempty keeps an unset version out of marshaled request bodies, where an explicit zero is rejected. Run \"gg gen\" to fill the tags in",
				typ, field.Name, strings.Join(quoted, " and ")))
		}
		info = versionField{index: i, column: versionColumnName(field), has: true}
		break
	}
	versionFieldCache.Store(typ, info)
	return info
}

// VersionGormTagMissing reports which of the two required gorm tag settings
// the Version field lacks. Matching is case-insensitive and
// whitespace-tolerant, but the requirement itself is exact: `not null` and
// `default:1` are the only shape the contract accepts (see the Version
// documentation for why the default is load-bearing).
func VersionGormTagMissing(tag reflect.StructTag) []string {
	hasNotNull, hasDefault := false, false
	for part := range strings.SplitSeq(tag.Get("gorm"), ";") {
		if strings.EqualFold(strings.Join(strings.Fields(part), " "), "not null") {
			hasNotNull = true
			continue
		}
		if key, value, ok := strings.Cut(part, ":"); ok &&
			strings.EqualFold(strings.TrimSpace(key), "default") && strings.TrimSpace(value) == "1" {
			hasDefault = true
		}
	}

	var missing []string
	if !hasNotNull {
		missing = append(missing, "not null")
	}
	if !hasDefault {
		missing = append(missing, "default:1")
	}
	return missing
}

// VersionJSONTagState classifies the Version field's json tag against the
// contract: the field must stay serializable (clients read the version they
// hand back) and must carry omitempty (an unset version on a marshaled
// struct would otherwise serialize as an explicit "version":0, which the
// write paths rightly reject). compliant reports whether the tag already
// satisfies both; healable is false only for json:"-", where adding
// omitempty cannot help and un-hiding the field is a semantic decision no
// tool should make.
func VersionJSONTagState(tag reflect.StructTag) (compliant, healable bool) {
	value, ok := tag.Lookup("json")
	if !ok {
		return false, true
	}
	name, options, _ := strings.Cut(value, ",")
	if strings.TrimSpace(name) == "-" && len(options) == 0 {
		return false, false
	}
	for option := range strings.SplitSeq(options, ",") {
		if strings.TrimSpace(option) == "omitempty" {
			return true, true
		}
	}
	return false, true
}

// versionJSONRequirement is the missing-setting label for a json tag that
// does not satisfy VersionJSONTagState, phrased as the fix.
const versionJSONRequirement = `json:",omitempty" serialization`

// VersionTagMissing reports every required tag setting the Version field
// lacks, gorm and json combined; the runtime enforcement panics on any.
func VersionTagMissing(tag reflect.StructTag) []string {
	missing := VersionGormTagMissing(tag)
	if compliant, _ := VersionJSONTagState(tag); !compliant {
		missing = append(missing, versionJSONRequirement)
	}
	return missing
}

// versionColumnName resolves the database column of the Version field the
// same way gorm does: an explicit column tag wins, the naming strategy
// renders the field name otherwise.
func versionColumnName(field reflect.StructField) string {
	for part := range strings.SplitSeq(field.Tag.Get("gorm"), ";") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(part), "column:"); ok && len(name) > 0 {
			return name
		}
	}
	return schema.NamingStrategy{}.ColumnName("", field.Name)
}

// IsVersioned reports whether m declares a Version field and therefore takes
// part in optimistic locking.
func IsVersioned(m any) bool { return versionFieldOf(m).has }

// VersionColumn reports the database column name of m's Version field, and
// whether m declares one.
func VersionColumn(m any) (string, bool) {
	info := versionFieldOf(m)
	return info.column, info.has
}

// VersionValue reports the version m carries. The second result is false for
// models without a Version field; a carried zero on a versioned model means
// "no version", per the type's contract.
func VersionValue(m any) (int64, bool) {
	info := versionFieldOf(m)
	if !info.has {
		return 0, false
	}
	value := reflect.ValueOf(m)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	return value.Field(info.index).Int(), true
}

// SetVersionValue writes v into m's Version field. It is a no-op for models
// without one or for values that cannot be addressed.
func SetVersionValue(m any, v int64) {
	info := versionFieldOf(m)
	if !info.has {
		return
	}
	value := reflect.ValueOf(m)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if !value.CanSet() {
		return
	}
	value.Field(info.index).SetInt(v)
}
