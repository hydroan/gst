package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
)

// This file holds how a patch arriving over HTTP names the fields it
// applies: by the keys of its JSON body (see patch_fields.go for the fields
// a patch may name).

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
// name, each key read the way the JSON decoder reads it (see lookup); a key
// no field encodes to names nothing and is left out, the way the decoder
// ignores an unknown key.
func patchFieldSetFromJSONFields(typ reflect.Type, fields map[string]json.RawMessage) patchFieldSet {
	if len(fields) == 0 {
		return patchFieldSet{}
	}
	table := patchFieldsOf(typ)
	fieldSet := make(patchFieldSet, len(fields))
	for key := range fields {
		if field, ok := table.lookup(key); ok {
			fieldSet[field.name] = struct{}{}
		}
	}
	return fieldSet
}
