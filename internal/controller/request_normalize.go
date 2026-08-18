package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
)

// This file keeps every bound request body service-safe and every body
// decoding failure client-safe. JSON binding alone guarantees neither: a
// literal JSON null body decodes into a nil pointer and then panics inside
// gin's validator step, null entries inside JSON arrays decode into nil slice
// elements, and decoder errors spell out Go struct and package internals.
// Phase services and the shared batch pipeline must never observe the former
// shapes, so every handler binds through bindJSONRequest and normalizes the
// bound value right after it succeeds; response envelopes must never carry
// the latter text, so every body-decoding entry point wraps its errors
// through clientSafeBindError.

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
// One knob does not carry over: gin's EnableDecoderUseNumber and
// EnableDecoderDisallowUnknownFields configure the streaming decoder only, so
// they never apply here.
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
		return clientSafeBindError(err)
	}
	// A nil binding.Validator is gin's documented way to turn validation off;
	// gin's own binding paths nil-check it, so binding here does the same.
	if binding.Validator == nil {
		return nil
	}
	if err = binding.Validator.ValidateStruct(target); err != nil {
		return clientSafeBindError(err)
	}
	return nil
}

// requiredBodyError translates the io.EOF sentinel of an absent request body
// into a client-safe rejection, for the model-path handlers that require a
// body: create, full update, and single-resource patch, where an empty body
// would fabricate, zero, or skip the whole resource. Handlers that tolerate
// an empty body keep treating io.EOF as "no body" and never call this.
// Non-EOF errors pass through unchanged — they were already wrapped at the
// decoding entry points.
func requiredBodyError(err error) error {
	if errors.Is(err, io.EOF) {
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, "request body is required", err)
	}
	return err
}

// clientSafeBindError wraps a request-body decoding or validation failure
// into a service-layer error whose client-safe message stays stable and free
// of implementation detail. Decoder errors spell out Go struct and package
// internals ("json: cannot unmarshal bool into Go struct field ..."), and the
// response envelope renders non-service errors verbatim, so wrapping at the
// decoding entry points is what keeps that text out of every bind failure at
// once. The original error stays wrapped as the cause: logs render the full
// decoder text through Error, io.EOF sentinels never reach this function
// (each entry point returns them before decoding), and type-mismatch field
// paths come from the target struct's JSON tags, not from client input.
func clientSafeBindError(err error) error {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return serviceregistry.NewErrorWithCause(http.StatusBadRequest, "invalid value for field '"+typeErr.Field+"'", err)
		}
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, "request body has an unexpected JSON type", err)
	case errors.As(err, &syntaxErr):
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, "request body is not valid JSON", err)
	default:
		return serviceregistry.NewErrorWithCause(http.StatusBadRequest, "invalid request body", err)
	}
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
