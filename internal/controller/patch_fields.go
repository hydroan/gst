package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
)

// This file holds how a patch names the fields it applies and how they are
// applied: over HTTP the keys of the JSON body name them, over gRPC the
// paths of the update mask, both by the JSON keys of the model; the fields
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
// in declaration order, with its lookups by JSON key and by Go path.
type patchFieldTable struct {
	fields []*patchField
	byKey  map[string]*patchField
	byName map[string]*patchField
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

// patchFieldTables caches the table of each model type: the table is read
// off the struct tags alone, so it is computed once per type and read-only
// from then on.
var patchFieldTables sync.Map // reflect.Type -> *patchFieldTable

// frameworkBases are the framework's base types, whose fields the framework
// manages and no patch applies.
var frameworkBases = map[reflect.Type]struct{}{
	reflect.TypeFor[modelregistry.Base]():     {},
	reflect.TypeFor[modelregistry.AutoBase](): {},
}

// patchFieldsOf returns the table of the fields of typ a patch may apply,
// computing it on the first call for the type: the fields the type encodes
// to JSON, under the keys it encodes them to, the fields of an embedded
// struct promoted to keys of the type's own the way encoding/json promotes
// them, and among two fields encoding to one key the one encoding/json
// picks, the shallower, or the tagged one at the same depth, and neither
// when still tied. Left out are the fields of the framework's base types,
// and a field named ID at any depth: the primary key names the record
// patched and moves it nowhere.
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
	// opaque are the index paths of the embedded structs encoding/json does
	// not look into, whose promoted fields are therefore no keys: the
	// framework's base types, an embedded struct with a json name of its
	// own, encoded as one value under it, one the tag leaves out, and one
	// held through a pointer of an unexported type.
	var opaque [][]int
	for _, field := range reflect.VisibleFields(typ) {
		if slices.ContainsFunc(opaque, func(prefix []int) bool {
			return len(field.Index) > len(prefix) && slices.Equal(field.Index[:len(prefix)], prefix)
		}) {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if field.Anonymous {
			structType := field.Type
			if structType.Kind() == reflect.Pointer {
				structType = structType.Elem()
			}
			if structType.Kind() == reflect.Struct {
				if _, base := frameworkBases[structType]; base || name == "-" || (field.Type.Kind() == reflect.Pointer && !field.IsExported()) {
					opaque = append(opaque, field.Index)
					continue
				}
				if name == "" {
					continue // the fields it promotes are the type's own
				}
				opaque = append(opaque, field.Index)
			}
		}
		if !field.IsExported() || name == "-" || field.Name == consts.FIELD_ID {
			continue
		}
		// A tag naming the field is a tag, spelt like the Go name or not.
		tagged := name != ""
		if !tagged {
			name = field.Name
		}
		if _, seen := candidates[name]; !seen {
			keys = append(keys, name)
		}
		var goPath []string
		for depth := range field.Index {
			goPath = append(goPath, typ.FieldByIndex(field.Index[:depth+1]).Name)
		}
		candidates[name] = append(candidates[name], candidate{
			field:  &patchField{key: name, name: strings.Join(goPath, "."), index: field.Index, typ: field.Type},
			tagged: tagged,
		})
	}
	table := &patchFieldTable{byKey: make(map[string]*patchField, len(keys)), byName: make(map[string]*patchField, len(keys))}
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

// add records field in the table.
func (t *patchFieldTable) add(field *patchField) {
	t.fields = append(t.fields, field)
	t.byKey[field.key] = field
	t.byName[field.name] = field
}

// applyPatch copies the fields of newVal that fieldSets name — every field
// of the table when none is given — onto oldVal, the record as stored, each
// as a whole: a struct value with the parts the request left at zero, a
// field promoted from an embedded struct through the struct, allocated on
// the way when held through a nil pointer; each copy is logged before it is
// made. The fields no patch applies never reach here (see patchFieldsOf),
// so the primary key names the record patched and the framework's base
// fields stay as stored.
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
		// output log must before set value.
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

// patchFieldSetFromJSONBody returns the fields of typ the keys of the JSON
// object body name, io.EOF for an empty body and a client-safe error for a
// malformed one.
func patchFieldSetFromJSONBody(typ reflect.Type, body []byte) (patchFieldSet, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return patchFieldSet{}, io.EOF
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, clientSafeBindError(err)
	}
	return patchFieldSetFromJSONFields(typ, fields), nil
}

// patchManyFieldSetsFromJSONBody returns the fields of typ each item of the
// JSON batch body names, one set per item in order, io.EOF for an empty
// body and a client-safe error for a malformed one.
func patchManyFieldSetsFromJSONBody(typ reflect.Type, body []byte) ([]patchFieldSet, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, io.EOF
	}
	var req struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, clientSafeBindError(err)
	}
	// A null item is dropped by the decoding of the batch (see
	// normalizeBatch), so it gets no field set either, keeping the sets
	// beside the items that remain.
	fieldSets := make([]patchFieldSet, 0, len(req.Items))
	for _, raw := range req.Items {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, clientSafeBindError(err)
		}
		fieldSets = append(fieldSets, patchFieldSetFromJSONFields(typ, item))
	}
	return fieldSets, nil
}

// patchFieldSetFromJSONFields returns the fields of typ the keys of fields
// name; a key no field encodes to names nothing and is left out, the way the
// JSON decoder ignores an unknown key.
func patchFieldSetFromJSONFields(typ reflect.Type, fields map[string]json.RawMessage) patchFieldSet {
	if len(fields) == 0 {
		return patchFieldSet{}
	}
	table := patchFieldsOf(typ)
	fieldSet := make(patchFieldSet, len(fields))
	for key := range fields {
		if field, ok := table.byKey[key]; ok {
			fieldSet[field.name] = struct{}{}
		}
	}
	return fieldSet
}

// maskFieldSet returns the fields of typ the paths of an update mask name,
// as the message names them, the JSON keys of the model: what a Patch rpc
// applies of the values it carries. The mask must name at least one field,
// a Patch applying nothing being a mistake to report rather than a
// record to answer unchanged, and every path must name a field the patch
// applies, as a whole: a path into a field, address.city, is refused and
// told the field to name instead, and so is a path naming what no patch
// applies, the framework's base fields, the primary key or a field the
// model does not have.
func maskFieldSet(typ reflect.Type, paths []string) (patchFieldSet, error) {
	if len(paths) == 0 {
		return nil, errors.New("update_mask must name at least one field")
	}
	table := patchFieldsOf(typ)
	fields := make(patchFieldSet, len(paths))
	for _, path := range paths {
		if head, _, into := strings.Cut(path, "."); into {
			if _, ok := table.byKey[head]; ok {
				return nil, errors.Newf("update_mask names %q, a part of a field; a patch applies %q as a whole", path, head)
			}
		}
		field, ok := table.byKey[path]
		if !ok {
			return nil, errors.Newf("update_mask names %q, which is no field a patch applies", path)
		}
		fields[field.name] = struct{}{}
	}
	return fields, nil
}
