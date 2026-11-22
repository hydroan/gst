package controller

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
)

// idFieldName is the name of the id field of a model, which a patch leaves
// alone.
const idFieldName = "ID"

// This file holds how a patch names the fields it applies and how they are
// applied: over HTTP the keys of the JSON body name them (see
// patch_fields_http.go), over gRPC the paths of the update mask (see
// patch_fields_grpc.go), both by the JSON keys of the model; the fields
// named are copied from the values the request carries onto the record as
// stored, each as a whole, whatever its type.

// patchField is one field a patch may apply: the JSON key naming it on
// either transport, the Go path naming it in a field set and to the
// validator, the index path reaching it through the embedded structs it is
// promoted from, and its type.
type patchField struct {
	key   string
	name  string
	index []int
	typ   reflect.Type
}

// patchFieldTable is the table of the fields of a model a patch may apply,
// in declaration order, with its lookups by JSON key and by Go path, the
// JSON keys of the fields the framework manages, which a mask names to no
// effect (see maskFieldSet), and the index paths of the fields among them
// the model declares read-only (see modelschema.IsReadOnly), which the
// create and update flows clear of what a client sent (see
// clearServerFields).
type patchFieldTable struct {
	fields   []*patchField
	byKey    map[string]*patchField
	byName   map[string]*patchField
	managed  map[string]struct{}
	readOnly [][]int
}

// lookup returns the field a JSON key names, the way encoding/json binds the
// value under the key: the field encoding to the key, else the first field
// in declaration order whose key differs from it only in case, so the field
// the decoder bound is the field the patch applies.
func (t *patchFieldTable) lookup(key string) (*patchField, bool) {
	if field, ok := t.byKey[key]; ok {
		return field, true
	}
	for _, field := range t.fields {
		if strings.EqualFold(field.key, key) {
			return field, true
		}
	}
	return nil, false
}

// add records field in the table.
func (t *patchFieldTable) add(field *patchField) {
	t.fields = append(t.fields, field)
	t.byKey[field.key] = field
	t.byName[field.name] = field
}

// patchFieldSet is the set of fields a patch applies, keyed by the Go path
// of each (see patchField).
type patchFieldSet map[string]struct{}

// covers reports whether the validator's name of a field, its Go path, is
// one the patch applies, a part of one, which the patch applies with it, or
// an embedded struct promoting one, which the validator looks into on its
// way there.
func (fields patchFieldSet) covers(name string) bool {
	if _, ok := fields[name]; ok {
		return true
	}
	for named := range fields {
		if strings.HasPrefix(name, named+".") || strings.HasPrefix(name, named+"[") || strings.HasPrefix(named, name+".") {
			return true
		}
	}
	return false
}

// versionOmitted reports whether fields, the set a patch of m applies,
// leaves out the Version m declares, and the Go field name of that Version
// when it does. A versioned model carries the version it was read with on
// every partial update, exactly like Update: the merge would otherwise keep
// the freshly loaded row's version and the lock would silently check the
// row against itself, so either transport refuses a patch omitting it. A
// model declaring no Version omits none. See modelschema.Version.
func versionOmitted(m any, fields patchFieldSet) (field string, omitted bool) {
	field, versioned := modelschema.VersionFieldName(m)
	if !versioned {
		return "", false
	}
	if _, named := fields[field]; named {
		return "", false
	}
	return field, true
}

// patchFieldTables caches the table of each model type: the table is read
// off the type alone, its struct tags and the columns gorm resolves from
// them, so it is computed once per type and read-only from then on.
var patchFieldTables sync.Map // reflect.Type -> *patchFieldTable

// patchFieldsOf returns the table of the fields of typ a patch may apply,
// computing it on the first call for the type: the fields the type encodes
// to JSON, under the keys it encodes them to, the fields of an embedded
// struct promoted to keys of the type's own the way encoding/json promotes
// them, and among two fields encoding to one key the one encoding/json
// picks, the shallower, or the tagged one at the same depth, and neither
// when still tied. The keys are what tell fields apart, as they do for
// encoding/json, not their Go names, which Go's own visibility goes by.
// Left out are the fields of the framework's base types, a field named ID at
// any depth, as the primary key names the record patched and moves it
// nowhere, a column an update leaves alone, one gorm writes on insert only
// or never, and a field the model declares read-only, which the server
// writes alone. Their keys are kept as the ones the framework manages.
func patchFieldsOf(typ reflect.Type) *patchFieldTable {
	if cached, ok := patchFieldTables.Load(typ); ok {
		return cached.(*patchFieldTable) //nolint:errcheck
	}
	type candidate struct {
		field  *patchField
		tagged bool
	}
	candidates := make(map[string][]candidate)
	var keys []string
	managed := make(map[string]struct{})
	var readOnly [][]int
	// The columns an update leaves alone, by index path: a patch applying one
	// would answer a value the write never takes (see
	// backfillUnwrittenColumns), so their keys are managed ones too.
	unwritten := make(map[string]struct{})
	if columns, err := modelschema.Columns(typ); err == nil {
		for _, col := range columns {
			if !col.Updatable {
				unwritten[fmt.Sprint(col.Index)] = struct{}{}
			}
		}
	}
	// The walk is encoding/json's typeFields, which it does not export:
	// breadth first, a level per depth of embedding, over every field a
	// struct holds, so that two fields Go hides behind one name are the two
	// keys they encode to. A struct type is looked into once, at the
	// shallowest depth it is embedded at, and a type embedded twice at one
	// depth has its fields counted twice, for the tie to take neither, the
	// way encoding/json takes neither.
	type embedding struct {
		typ   reflect.Type
		index []int
	}
	current := []embedding{{typ: typ}}
	count := map[reflect.Type]int{typ: 1}
	visited := map[reflect.Type]bool{}
	for len(current) > 0 {
		var next []embedding
		nextCount := map[reflect.Type]int{}
		for _, e := range current {
			if visited[e.typ] {
				continue
			}
			visited[e.typ] = true
			for i := range e.typ.NumField() {
				field := e.typ.Field(i)
				name := jsonTagName(field)
				index := append(slices.Clone(e.index), i)
				if field.Anonymous {
					structType := field.Type
					if structType.Kind() == reflect.Pointer {
						structType = structType.Elem()
					}
					if structType.Kind() == reflect.Struct {
						// A framework base type is not looked into: its
						// fields are the framework's to write, their keys
						// kept as the managed ones.
						if modelregistry.IsFrameworkBase(structType) {
							for baseField := range structType.Fields() {
								if key := jsonKey(baseField); key != "" {
									managed[key] = struct{}{}
								}
							}
							continue
						}
						// Neither is an embedded struct with a json name of
						// its own, encoded as one value under it, one the tag
						// leaves out, or one held through a pointer of an
						// unexported type: their promoted fields are no keys.
						if name == "-" || (field.Type.Kind() == reflect.Pointer && !field.IsExported()) {
							continue
						}
						if name == "" {
							nextCount[structType]++
							if nextCount[structType] == 1 {
								next = append(next, embedding{typ: structType, index: index})
							}
							continue // the fields it promotes are the type's own
						}
					}
				}
				if !field.IsExported() || name == "-" {
					continue
				}
				if modelschema.IsReadOnly(field.Tag) {
					managed[jsonKey(field)] = struct{}{}
					readOnly = append(readOnly, index)
					continue
				}
				if _, alone := unwritten[fmt.Sprint(index)]; alone || field.Name == idFieldName {
					managed[jsonKey(field)] = struct{}{}
					continue
				}
				// A tag naming the field is a tag, spelt like the Go name or
				// not.
				tagged := name != ""
				if !tagged {
					name = field.Name
				}
				if _, seen := candidates[name]; !seen {
					keys = append(keys, name)
				}
				var goPath []string
				for depth := range index {
					goPath = append(goPath, typ.FieldByIndex(index[:depth+1]).Name)
				}
				c := candidate{
					field:  &patchField{key: name, name: strings.Join(goPath, "."), index: index, typ: field.Type},
					tagged: tagged,
				}
				candidates[name] = append(candidates[name], c)
				if count[e.typ] > 1 {
					candidates[name] = append(candidates[name], c)
				}
			}
		}
		current, count = next, nextCount
	}
	table := &patchFieldTable{byKey: make(map[string]*patchField, len(keys)), byName: make(map[string]*patchField, len(keys)), managed: managed, readOnly: readOnly}
	for _, key := range keys {
		shallowest := slices.MinFunc(candidates[key], func(a, b candidate) int {
			return len(a.field.index) - len(b.field.index)
		})
		var tied, tagged []candidate
		for _, c := range candidates[key] {
			if len(c.field.index) == len(shallowest.field.index) {
				tied = append(tied, c)
				if c.tagged {
					tagged = append(tagged, c)
				}
			}
		}
		switch {
		case len(tied) == 1:
			table.add(tied[0].field)
		case len(tagged) == 1:
			table.add(tagged[0].field)
		}
	}
	// The declaration order, which the candidates lost on their way through
	// the keys.
	slices.SortFunc(table.fields, func(a, b *patchField) int { return slices.Compare(a.index, b.index) })
	patchFieldTables.Store(typ, table)
	return table
}

// jsonKey returns the key field encodes to in JSON, the name its json tag
// gives it or its own name, and "" for a field encoding to none, unexported
// or left out by its tag.
func jsonKey(field reflect.StructField) string {
	name := jsonTagName(field)
	if !field.IsExported() || name == "-" {
		return ""
	}
	if name == "" {
		return field.Name
	}
	return name
}

// applyPatch copies the fields of newVal that fieldSets name — every field
// of the table when none is given — onto oldVal, the record as stored, each
// as a whole: a struct value with the parts the request left at zero, a
// field promoted from an embedded struct through the struct, allocated on
// the way when held through a nil pointer; each copy is logged before it is
// made. The fields no patch applies never reach here (see patchFieldsOf),
// so the primary key names the record patched and the framework's base
// fields and the read-only ones stay as stored.
func applyPatch(log types.Logger, typ reflect.Type, oldVal reflect.Value, newVal reflect.Value, fieldSets ...patchFieldSet) {
	var fields patchFieldSet
	if len(fieldSets) > 0 {
		fields = fieldSets[0]
	}
	for _, field := range patchFieldsOf(typ).fields {
		if fields != nil {
			if _, ok := fields[field.name]; !ok {
				continue
			}
		}
		target := fieldByIndexAllocating(oldVal, field.index)
		if !target.CanSet() {
			log.Debugz("field cannot be set, skip", zap.String("field", field.name))
			continue
		}
		value, err := newVal.FieldByIndexErr(field.index)
		if err != nil {
			// The request left the embedded struct promoting the field nil,
			// so the field it carries is the zero value.
			value = reflect.Zero(field.typ)
		}
		// The copy is logged before it is made.
		log.Infof("[PATCH %s] field: %q: %v --> %v", typ.Name(), field.name, patchValue(target), patchValue(value))
		target.Set(value)
	}
}

// fieldByIndexAllocating is v.FieldByIndex(index) with the nil pointers on
// the way allocated, where they can be set, so that a field promoted
// through an embedded pointer can be set; the invalid Value where one
// cannot.
func fieldByIndexAllocating(v reflect.Value, index []int) reflect.Value {
	for depth, i := range index {
		if depth > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

// patchValue is the value of v for the patch log: what a pointer points to,
// <nil> for a nil one.
func patchValue(v reflect.Value) any {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "<nil>"
		}
		return v.Elem().Interface()
	}
	return v.Interface()
}
