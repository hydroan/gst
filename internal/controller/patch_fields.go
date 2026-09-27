package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
)

// This file holds how a patch names the fields it applies and how they are
// applied: over HTTP the keys of the JSON body name them, over gRPC the
// paths of the update mask, both by the JSON keys of the model; the fields
// named are copied from the values the request carries onto the record as
// stored.

// patchFieldSet is the set of fields a patch applies, keyed by Go field
// name.
type patchFieldSet map[string]struct{}

// applyPatch copies the fields of newVal that fieldSets name — every field
// when none is given — onto oldVal, the record as stored: the model's own
// fields only, the framework's base fields and any other struct-kind field
// being left alone, and each copy logged before it is made.
func applyPatch(log types.Logger, typ reflect.Type, oldVal reflect.Value, newVal reflect.Value, fieldSets ...patchFieldSet) {
	var fields patchFieldSet
	if len(fieldSets) > 0 {
		fields = fieldSets[0]
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		if fields != nil {
			if _, ok := fields[field.Name]; !ok {
				continue
			}
		}
		if field.Type.Kind() == reflect.Struct { // skip update base model.
			// Base and AutoBase contain framework-managed fields and should not
			// be patched directly; other nested struct fields are skipped from
			// patching as well.
			continue
		}
		if !oldVal.Field(i).CanSet() {
			log.Debugz("field cannot be set, skip", zap.String("field", field.Name))
			continue
		}
		if !newVal.Field(i).IsValid() {
			continue
		}
		// output log must before set value.
		if newVal.Field(i).Kind() == reflect.Pointer {
			var oldValue, newValue any
			if !oldVal.Field(i).IsNil() {
				oldValue = oldVal.Field(i).Elem().Interface()
			} else {
				oldValue = "<nil>"
			}
			if !newVal.Field(i).IsNil() {
				newValue = newVal.Field(i).Elem().Interface()
			} else {
				newValue = "<nil>"
			}
			log.Infof("[PATCH %s] field: %q: %v --> %v", typ.Name(), field.Name, oldValue, newValue)
		} else {
			log.Infof("[PATCH %s] field: %q: %v --> %v", typ.Name(), field.Name, oldVal.Field(i).Interface(), newVal.Field(i).Interface())
		}
		oldVal.Field(i).Set(newVal.Field(i)) // set old value by new value
	}
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
	jsonFields := patchJSONFieldNames(typ)
	fieldSet := make(patchFieldSet, len(fields))
	for name := range fields {
		if fieldName, ok := jsonFields[name]; ok {
			fieldSet[fieldName] = struct{}{}
		}
	}
	return fieldSet
}

// maskFieldSet returns the fields of typ the paths of an update mask name,
// as the message names them, the JSON keys of the model: what a Patch rpc
// applies of the values it carries. The mask must name at least one field,
// a Patch applying nothing being a mistake to report rather than a
// record to answer unchanged, and every path must name a field the patch
// can apply: the model's own fields, not the framework's base fields, a
// nested struct or a field the model does not have.
func maskFieldSet(typ reflect.Type, paths []string) (patchFieldSet, error) {
	if len(paths) == 0 {
		return nil, errors.New("update_mask must name at least one field")
	}
	jsonFields := patchJSONFieldNames(typ)
	kinds := cachedModelFieldKinds(typ)
	fields := make(patchFieldSet, len(paths))
	for _, path := range paths {
		fieldName, ok := jsonFields[path]
		if !ok || kinds[fieldName] == reflect.Struct {
			return nil, errors.Newf("update_mask names %q, which is no field a patch applies", path)
		}
		fields[fieldName] = struct{}{}
	}
	return fields, nil
}

// patchJSONFieldNamesCache caches the JSON-name-to-field-name mapping per
// model type. The mapping is derived from struct tags only, so it is computed
// once per type instead of on every patch request; cached maps are read-only.
var patchJSONFieldNamesCache sync.Map // reflect.Type -> map[string]string

// patchJSONFieldNames returns the Go field name of typ each JSON key names,
// computing the mapping on the first call for the type and caching it.
func patchJSONFieldNames(typ reflect.Type) map[string]string {
	if cached, ok := patchJSONFieldNamesCache.Load(typ); ok {
		return cached.(map[string]string) //nolint:errcheck
	}
	fields := make(map[string]string, typ.NumField())
	for field := range typ.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, ok := patchJSONFieldName(field)
		if !ok {
			continue
		}
		fields[name] = field.Name
	}
	patchJSONFieldNamesCache.Store(typ, fields)
	return fields
}

// patchJSONFieldName returns the JSON key field encodes to, the json tag's
// name or the field name without one, and false for a field the tag leaves
// out.
func patchJSONFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		tag = tag[:comma]
	}
	if tag != "" {
		return tag, true
	}
	return field.Name, true
}
