package controller

import (
	"bytes"
	"io"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/hydroan/gst/types"
)

// This file keeps every bound request body service-safe. JSON binding alone
// cannot guarantee that: a literal JSON null body decodes into a nil pointer
// and then panics inside gin's validator step, and null entries inside JSON
// arrays decode into nil slice elements. Phase services and the shared batch
// pipeline must never observe either shape, so every handler binds through
// bindJSONRequest and normalizes the bound value right after it succeeds.

// jsonNull is the literal JSON null body treated as "no body".
var jsonNull = []byte("null")

// bindJSONRequest binds the JSON request body into target. A body that is
// empty or a literal JSON null carries no request data, so both report io.EOF
// — the sentinel the handlers already tolerate for empty bodies. This also
// keeps the null body away from gin's validator, which panics on the nil
// pointer such a body would decode into.
//
// The body is decoded from the bytes already read rather than handed back to
// gin as a reader: binding through gin wraps those bytes in a reader and drives
// a streaming decoder across them, paying for a decoder and its buffer to
// re-read what is already in memory. Decoding still goes through gin's codec
// and validation through gin's validator, so an application that swapped the
// JSON implementation keeps that choice here, and a bound request is checked
// exactly as gin would check it. The body is put back either way — reading it
// here must not stop anything downstream from reading it again.
//
// Decoding whole bytes also ends the body where the body ends: a streaming
// decoder stops at the first JSON value and silently drops whatever follows,
// so a second document appended to the first would bind as if it were clean.
func bindJSONRequest(c *gin.Context, target any) error {
	raw, err := c.GetRawData()
	if err != nil {
		return err
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || bytes.Equal(trimmed, jsonNull) {
		return io.EOF
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))

	if err = ginjson.API.Unmarshal(raw, target); err != nil {
		return err
	}
	return binding.Validator.ValidateStruct(target)
}

// normalizeRequest restores req to the zero-value instance when a JSON null
// body left it nil — indistinguishable from an empty body for the service —
// and compacts nil slice elements away.
func (meta *factoryMeta[M, REQ, RSP]) normalizeRequest(req *REQ) {
	if meta.reqKind == reflect.Pointer && reflect.ValueOf(*req).IsNil() {
		*req = meta.newRequest()
	}
	compactNilSliceElements(reflect.ValueOf(req))
}

// normalizeModel is the model-path counterpart of normalizeRequest for
// handlers that bind the request body straight into the model type. Model
// types are pointers by construction, so only the nil restore and the slice
// compaction apply.
func (meta *factoryMeta[M, REQ, RSP]) normalizeModel(m *M) {
	if reflect.ValueOf(*m).IsNil() {
		*m = meta.newModel()
	}
	compactNilSliceElements(reflect.ValueOf(m))
}

// normalizeBatchRequest compacts nil entries out of the bound batch payload
// so the shared batch pipeline never dereferences a nil item.
func normalizeBatchRequest[M types.Model](req *requestData[M]) {
	compactNilSliceElements(reflect.ValueOf(req))
}

// nilableKind reports whether values of kind k can hold nil, i.e. whether a
// JSON null entry can decode into them.
func nilableKind(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return true
	default:
		return false
	}
}

// compactNilSliceElements walks the value graph reachable from v and removes
// nil elements from every settable slice whose elements can hold nil. Only
// exported struct fields are visited, matching what encoding/json can bind.
// JSON-decoded values are acyclic, so the walk needs no cycle tracking.
// Interface values are descended into only when they carry a pointer: other
// interface payloads are not addressable, so their inner slices cannot be
// compacted in place and are left untouched.
func compactNilSliceElements(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			compactNilSliceElements(v.Elem())
		}
	case reflect.Interface:
		if !v.IsNil() && v.Elem().Kind() == reflect.Pointer {
			compactNilSliceElements(v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			compactNilSliceElements(v.Field(i))
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		if nilableKind(v.Type().Elem().Kind()) && v.CanSet() {
			kept := 0
			for i := range v.Len() {
				if v.Index(i).IsNil() {
					continue
				}
				if kept != i {
					v.Index(kept).Set(v.Index(i))
				}
				kept++
			}
			if kept != v.Len() {
				v.SetLen(kept)
			}
		}
		for i := range v.Len() {
			compactNilSliceElements(v.Index(i))
		}
	case reflect.Array:
		for i := range v.Len() {
			compactNilSliceElements(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() {
			return
		}
		for _, key := range v.MapKeys() {
			value := v.MapIndex(key)
			tmp := reflect.New(value.Type()).Elem()
			tmp.Set(value)
			compactNilSliceElements(tmp)
			v.SetMapIndex(key, tmp)
		}
	default:
	}
}
