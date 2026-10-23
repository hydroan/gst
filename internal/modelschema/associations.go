package modelschema

import (
	"reflect"
	"slices"
	"sync"

	gormschema "gorm.io/gorm/schema"
)

// associationFieldsCache memoizes the association field paths per model type.
var associationFieldsCache sync.Map

// AssociationFields reports the struct field index paths of a model type's
// associations — the relationships its gorm schema holds for the type itself,
// a has-many or a belongs-to for one, declared for queries to preload —
// resolved once per type and cached, in field order; nil for a type whose
// schema cannot be resolved. A path is the one reflect walks
// (reflect.Value.FieldByIndexErr), a step through a pointer-embedded struct
// included. Its consumer is the write path, which zeroes them: a write
// touches the model's own table alone.
func AssociationFields(typ reflect.Type) [][]int {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := associationFieldsCache.Load(typ); ok {
		return cached.([][]int) //nolint:errcheck
	}

	paths := [][]int{}
	if !walkOwnRelationships(typ, func(_ string, rel *gormschema.Relationship, _ *gormschema.Schema) {
		// gorm's schema writes a step through a pointer-embedded struct as
		// -i-1 for the field i it takes, which reflect takes as i.
		path := slices.Clone(rel.Field.StructField.Index)
		for i, step := range path {
			if step < 0 {
				path[i] = -step - 1
			}
		}
		paths = append(paths, path)
	}) {
		paths = nil
	}
	slices.SortFunc(paths, slices.Compare)
	associationFieldsCache.Store(typ, paths)
	return paths
}

// recursiveAssociationsCache memoizes the recursive associations per model
// type.
var recursiveAssociationsCache sync.Map

// RecursiveAssociations reports, by field name, the to-many associations of
// a model type that hold the type itself — gorm's self-referential has-many
// and many-to-many, the Children of a tree — resolved once per type and
// cached; nil for a type whose schema cannot be resolved. Its consumer is the
// depth of an expanding request, which repeats only these: the rows of any
// other association are of a model with no such association to descend
// into.
func RecursiveAssociations(typ reflect.Type) map[string]struct{} {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := recursiveAssociationsCache.Load(typ); ok {
		return cached.(map[string]struct{}) //nolint:errcheck
	}

	names := map[string]struct{}{}
	if !walkOwnRelationships(typ, func(name string, rel *gormschema.Relationship, parsed *gormschema.Schema) {
		if (rel.Type == gormschema.HasMany || rel.Type == gormschema.Many2Many) && rel.FieldSchema == parsed {
			names[name] = struct{}{}
		}
	}) {
		names = nil
	}
	recursiveAssociationsCache.Store(typ, names)
	return names
}

// walkOwnRelationships calls visit, under the schema lock, for every
// relationship gorm's schema of typ holds for typ itself, and reports false
// when typ is no struct gorm resolves a schema for. gorm writes into these
// relationships under that lock when it parses a type holding typ, so the
// walk takes it too, or a parse running beside it could hand it an entry half
// written. Once gorm parses a type holding typ through a has-one or a
// has-many, it registers that relationship on typ's schema too, under a
// hidden name, with the holder's field; gorm's own preload of
// clause.Associations keeps the relationships whose schema is the type's, and
// so does this walk.
func walkOwnRelationships(typ reflect.Type, visit func(name string, rel *gormschema.Relationship, parsed *gormschema.Schema)) bool {
	if typ == nil || typ.Kind() != reflect.Struct {
		return false
	}
	parsed, err := gormschema.Parse(reflect.New(typ).Interface(), schemaCache, namer)
	if err != nil {
		return false
	}
	parsed.Relationships.Mux.RLock()
	defer parsed.Relationships.Mux.RUnlock()
	for name, rel := range parsed.Relationships.Relations {
		if rel.Schema == parsed {
			visit(name, rel, parsed)
		}
	}
	return true
}
