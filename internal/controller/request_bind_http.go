package controller

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/gst"
)

// This file keeps every bound request body service-safe and every body
// decoding failure client-safe. JSON binding alone guarantees neither: a
// literal JSON null body decodes into a nil pointer and then panics inside
// gin's validator step, null entries inside JSON arrays decode into nil slice
// elements, and decoder errors spell out Go struct and package internals.
// Phase services and the shared batch pipeline must never observe the former
// shapes, so every handler binds through bindJSONRequest, which normalizes
// the bound value before validating it; response envelopes must never carry
// the latter text, so every body-decoding entry point wraps its errors
// through clientSafeBindError.

// jsonNull is the literal JSON null body treated as "no body".
var jsonNull = []byte("null")

// bindJSONRequest decodes the JSON request body into target (see
// decodeJSONRequest), normalizes what it holds (see normalizeValue) and
// validates it against its binding tags (see validateRequest), in that
// order, so the body is validated as the service receives it, the way the
// gRPC calls validate a message: what every handler binding a body does, but
// the patch handlers, which validate the fields the body names alone once
// they know which (see validatePatchFields).
func bindJSONRequest(c *gin.Context, target any) error {
	if err := decodeJSONRequest(c, target); err != nil {
		return err
	}
	normalizeValue(reflect.ValueOf(target))
	if err := validateRequest(target); err != nil {
		return clientSafeBindError(err)
	}
	return nil
}

// decodeJSONRequest decodes the JSON request body into target, read through
// readJSONRequestBody, so a body failing to read is refused client-safely
// like any bind failure. A body that is empty or a literal JSON null carries
// no request data, so both report io.EOF — the sentinel a handler requiring
// a body answers with its refusal (see requiredBodyError) and one whose
// request may carry none reads as "no body". This also keeps the null body
// away from gin's validator, which panics on the nil pointer such a body
// would decode into.
//
// The body is decoded from the bytes already read rather than handed back to
// gin as a reader: binding through gin wraps those bytes in a reader and drives
// a streaming decoder across them, paying for a decoder and its buffer to
// re-read what is already in memory. Decoding goes through encoding/json
// whatever JSON codec gin was built with: the codecs gin's jsoniter, go_json
// and sonic build tags select decode differently, the framework's wire
// contract is the encoding/json one, and clientSafeBindError translates
// encoding/json's error types. Validation is the caller's, through gin's
// validator (see validateRequest and validatePatchFields), so a bound request
// is checked exactly as gin would check it. The body is put back either way —
// reading it here must not stop anything downstream from reading it again.
//
// Decoding whole bytes also ends the body where the body ends: a streaming
// decoder stops at the first JSON value and silently drops whatever follows,
// so a second document appended to the first would bind as if it were clean.
// One knob does not carry over: gin's EnableDecoderUseNumber and
// EnableDecoderDisallowUnknownFields configure the streaming decoder only, so
// they never apply here. A time of day is the one value decoded otherwise
// than json.Unmarshal decodes it, and a time, a day or a number it refuses is
// refused naming the field (see requestDecoding).
func decodeJSONRequest(c *gin.Context, target any) error {
	raw, err := readJSONRequestBody(c)
	if err != nil {
		return err
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || bytes.Equal(trimmed, jsonNull) {
		return io.EOF
	}
	return decodeJSON(raw, target)
}

// decodeJSON decodes raw, one JSON document, into target under the options a
// request body is decoded under (see decodeJSONRequest and requestDecoding),
// a failure wrapped client-safely (see clientSafeBindError): the decoding of
// a request body and of a message of a stream over HTTP alike.
func decodeJSON(raw []byte, target any) error {
	if err := jsonv2.Unmarshal(raw, target, json.DefaultOptionsV1(), requestDecoding); err != nil {
		return clientSafeBindError(err)
	}
	return nil
}

// requestDecoding is the decoding option a request body is decoded under
// beside the options json.DefaultOptionsV1 names. json.Unmarshal is
// encoding/json/v2 under those options, so a body decodes the way
// json.Unmarshal decodes it, every value reached the way it reaches it, with
// one difference: a time.Time, a json.Number, a day or a time of day
// json.Unmarshal refuses is refused naming the field (see decodeAtPointer
// and ownTextAtPointer).
var requestDecoding = jsonv2.WithUnmarshalers(jsonv2.JoinUnmarshalers(
	jsonv2.UnmarshalFromFunc(ownTextAtPointer[datatypes.Date]),
	jsonv2.UnmarshalFromFunc(ownTextAtPointer[datatypes.TimeOfDay]),
	jsonv2.UnmarshalFromFunc(decodeAtPointer[time.Time]),
	jsonv2.UnmarshalFromFunc(decodeAtPointer[json.Number]),
))

// decodeAtPointer decodes the value the decoder is at into v exactly as
// json.Unmarshal decodes it, under the options json.DefaultOptionsV1 names, and
// reports a value it refuses as a SemanticError at the value's JSON Pointer,
// holding the value. Those options lose the field of a time.Time or a
// json.Number they refuse: the parse error of a time comes back bare, a
// *time.ParseError, and the refusal of a number is an UnmarshalTypeError naming
// no field. At its pointer the refusal is reported as a json.UnmarshalTypeError
// naming the field, which clientSafeBindError answers with
// "invalid value for field 'due_at'", the way a generated FromProto refuses the
// value over gRPC (see grpc.Time and grpc.Number). A JSON number or a null is
// left to the default decoding, through errors.ErrUnsupported: it names the
// field of what it refuses, and it honors the field's own tag options, which
// this function never sees, a json.Number with the string option taking no
// bare number. The gRPC stand-in of the contract table, fromProto in
// call_grpc_test.go, decodes through a copy of this function: a change to
// one is a change to the other.
func decodeAtPointer[T any](dec *jsontext.Decoder, v *T) error {
	if kind := dec.PeekKind(); kind == '0' || kind == 'n' {
		return errors.ErrUnsupported
	}
	value, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if err = jsonv2.Unmarshal(value, v, json.DefaultOptionsV1()); err != nil {
		return &jsonv2.SemanticError{JSONPointer: dec.StackPointer(), JSONKind: value.Kind(), JSONValue: value.Clone(), GoType: reflect.TypeFor[T](), Err: err}
	}
	return nil
}

// ownTextAtPointer reads a value of a type that reads its own text from
// JSON, a datatypes.Date or a datatypes.TimeOfDay, through the type's
// UnmarshalJSON: a string in time.DateOnly or time.TimeOnly and nothing
// else, a null leaving the value as it is. It reports a value the type
// refuses as a SemanticError at the value's JSON Pointer. Left to the type's
// own UnmarshalJSON the refusal comes back bare, naming no field; at its
// pointer the options json.DefaultOptionsV1 names report it as a
// json.UnmarshalTypeError naming the field, items.1.day, which
// clientSafeBindError answers the way it answers a time it refuses. A
// number is read here too, unlike decodeAtPointer leaves one: no day and no
// time of day is a number, so the default decoding has nothing of its own
// to say about it.
func ownTextAtPointer[T any, P interface {
	*T
	json.Unmarshaler
}](dec *jsontext.Decoder, v P) error {
	value, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if err = v.UnmarshalJSON(value); err != nil {
		return &jsonv2.SemanticError{JSONPointer: dec.StackPointer(), JSONKind: value.Kind(), JSONValue: value.Clone(), GoType: reflect.TypeFor[T](), Err: err}
	}
	return nil
}

// requiredBodyError translates the io.EOF sentinel of an absent request body
// into a client-safe rejection, for the model-path handlers that require a
// body: the create, the full update and the patch of one resource, where an
// empty body would fabricate, zero, or skip the whole resource, and the
// batch create, update, patch and delete, where it would act on nothing. A
// handler whose request may carry no body keeps treating io.EOF as "no
// body" and never calls this. Non-EOF errors pass through unchanged — they
// were already wrapped at the decoding entry points.
func requiredBodyError(err error) error {
	if errors.Is(err, io.EOF) {
		return gst.NewErrorWithCause(http.StatusBadRequest, missingBodyMsg, err)
	}
	return err
}

// clientSafeBindError wraps a request-body decoding or validation failure
// into a service-layer error whose client-safe message stays stable and free
// of implementation detail. Decoder errors spell out Go struct and package
// internals ("json: cannot unmarshal bool into Go struct field ..."), and the
// response envelope renders non-service errors verbatim, so wrapping at the
// decoding entry points is what keeps that text out of every bind failure at
// once. A value a field cannot hold is refused naming the field (see
// gst.NewInvalidValue) by the path the contract spells, items[1].rank
// (see indexedJSONPath), the way a generated FromProto refuses the value
// over gRPC. The original error stays wrapped as the cause: logs render the
// full decoder text through Error, io.EOF sentinels never reach this
// function (each entry point returns them before decoding), and
// type-mismatch field paths come from the target struct's JSON tags, not
// from client input.
func clientSafeBindError(err error) error {
	if violations := fieldViolations(err, ""); len(violations) > 0 {
		return gst.NewInvalidFields(violations, err)
	}
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return gst.NewInvalidValue(indexedJSONPath(typeErr.Field), typeErr.Value+" does not fit the field", err)
		}
		return gst.NewErrorWithCause(http.StatusBadRequest, unexpectedJSONTypeMsg, err)
	case errors.As(err, &syntaxErr):
		return gst.NewErrorWithCause(http.StatusBadRequest, invalidJSONMsg, err)
	default:
		return gst.NewErrorWithCause(http.StatusBadRequest, invalidBodyMsg, err)
	}
}

// indexedJSONPath spells path, the field path of a json.UnmarshalTypeError,
// the way the contract spells it: encoding/json joins every step with a
// dot, items.1.rank, and a step that is an index is written in brackets
// instead, items[1].rank, as the validator's paths are (see jsonFieldPath)
// and as the gRPC side names a refused value (see pathStep).
func indexedJSONPath(path string) string {
	spelled := ""
	for step := range strings.SplitSeq(path, ".") {
		spelled = pathStep(spelled, step)
	}
	return spelled
}

// clientSafeItemBindError is clientSafeBindError for the item at index i of
// a batch validated on its own, the fields it names carrying the item in
// front, items[1].name.
func clientSafeItemBindError(i int, err error) error {
	if violations := fieldViolations(err, itemPrefix(i)); len(violations) > 0 {
		return gst.NewInvalidFields(violations, err)
	}
	return clientSafeBindError(err)
}
