package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	entranslations "github.com/go-playground/validator/v10/translations/en"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
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

// bindJSONRequest decodes the JSON request body into target (see
// decodeJSONRequest) and validates it against its binding tags (see
// validateRequest): what every handler binding a body does, but the patch
// handlers, which validate the fields the body names alone once they know
// which (see validatePatchFields).
func bindJSONRequest(c *gin.Context, target any) error {
	if err := decodeJSONRequest(c, target); err != nil {
		return err
	}
	if err := validateRequest(target); err != nil {
		return clientSafeBindError(err)
	}
	return nil
}

// decodeJSONRequest decodes the JSON request body into target. A body that is
// empty or a literal JSON null carries no request data, so both report io.EOF
// — the sentinel the handlers already tolerate for empty bodies. This also
// keeps the null body away from gin's validator, which panics on the nil
// pointer such a body would decode into.
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
// they never apply here.
func decodeJSONRequest(c *gin.Context, target any) error {
	raw, err := c.GetRawData()
	if err != nil {
		return err
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || bytes.Equal(trimmed, jsonNull) {
		return io.EOF
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err = json.Unmarshal(raw, target); err != nil {
		return clientSafeBindError(err)
	}
	return nil
}

// validateRequest checks target against its binding tags with gin's
// validator: the check bindJSONRequest makes of a bound body and the call
// functions make of the value a request message decoded into, so a model or
// payload meets the same tags on both transports. The error is the
// validator's own, naming Go fields, for each transport to wrap in its
// client-safe message. A nil binding.Validator is gin's documented way to
// turn validation off; gin's own binding paths nil-check it, so this does
// the same.
func validateRequest(target any) error {
	if binding.Validator == nil {
		return nil
	}
	return binding.Validator.ValidateStruct(target)
}

var (
	// validatorEngine is gin's validator as configured at initialization
	// (see init), whose errors fieldViolations names fields of; nil when
	// gin's validator is not go-playground's.
	validatorEngine *validator.Validate
	// validatorTranslator renders the errors of validatorEngine as English
	// sentences (see fieldViolations).
	validatorTranslator ut.Translator
)

// init sets gin's validator up for the answers the framework gives, at
// package initialization so that it is set before any request, on either
// transport, and before any validation a project runs through gin itself:
// the validator names fields by their JSON key, the name the client sent
// them under, and renders each failure as the English sentence of its own
// translations, "name is a required field". A validator other than
// go-playground's is left as it is, its errors answered without naming a
// field (see clientSafeBindError).
func init() {
	if binding.Validator == nil {
		return
	}
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	engine.RegisterTagNameFunc(func(field reflect.StructField) string {
		if name := jsonTagName(field); name != "-" {
			return name
		}
		return ""
	})
	english := en.New()
	translator, _ := ut.New(english, english).GetTranslator(english.Locale())
	if err := entranslations.RegisterDefaultTranslations(engine, translator); err != nil {
		return
	}
	validatorEngine = engine
	validatorTranslator = translator
}

// jsonTagName returns the name a field's json tag gives it: the part before
// the comma, "-" for a field the tag leaves out, and "" for a field without
// a name in its tag, which encoding/json names after the field.
func jsonTagName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	return name
}

// fieldViolations returns the fields a validator error names, one violation
// per field: the field by its JSON key path relative to the request, with
// prefix in front, items[1]. for the item of a batch validated on its own,
// and the sentence the validator's translation renders the failure as,
// with the path in place of the bare field name; nil for any other error,
// and for the errors of a validator other than the one init configured,
// which a project may have put in gin's place since.
func fieldViolations(err error, prefix string) []serviceregistry.FieldViolation {
	var refused validator.ValidationErrors
	if validatorEngine == nil || binding.Validator == nil || binding.Validator.Engine() != validatorEngine || !errors.As(err, &refused) {
		return nil
	}
	violations := make([]serviceregistry.FieldViolation, 0, len(refused))
	for _, fe := range refused {
		path := prefix + fieldPath(fe.Namespace())
		description := fe.Translate(validatorTranslator)
		if field := fe.Field(); path != field {
			description = path + strings.TrimPrefix(description, field)
		}
		violations = append(violations, serviceregistry.FieldViolation{Field: path, Description: description})
	}
	return violations
}

// fieldPath returns the namespace of a validator error without the type of
// the request at its head: what follows the first dot outside the brackets
// a generic type's name may carry, batch[...].items[1].name being the
// namespace of an item's field.
func fieldPath(namespace string) string {
	depth := 0
	for i, r := range namespace {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				return namespace[i+1:]
			}
		}
	}
	return namespace
}

// validatePatchFields checks target against the binding tags of the fields
// that fields names and of no other, with gin's validator: a patch carries
// the fields it changes, so a tag on a field it leaves out, required above
// all, is not its to meet, on either transport. A field named is checked
// as the whole the patch applies, the tags inside a struct value included;
// the validator names each field by its Go path under the type's name,
// which is how patchFieldSet keys the fields (see covers). Nothing named
// validates nothing. A validator other than go-playground's cannot be asked
// for a part of the struct and checks the whole; nil turns validation off,
// see validateRequest.
func validatePatchFields(target any, fields patchFieldSet) error {
	if binding.Validator == nil || len(fields) == 0 {
		return nil
	}
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return binding.Validator.ValidateStruct(target)
	}
	typ := reflect.TypeOf(target)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	prefix := typ.Name() + "."
	return engine.StructFiltered(target, func(ns []byte) bool {
		return !fields.covers(strings.TrimPrefix(string(ns), prefix))
	})
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
	if violations := fieldViolations(err, ""); len(violations) > 0 {
		return serviceregistry.NewInvalidFields(violations, err)
	}
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

// clientSafeItemBindError is clientSafeBindError for the item at index i of
// a batch validated on its own, the fields it names carrying the item in
// front, items[1].name.
func clientSafeItemBindError(i int, err error) error {
	if violations := fieldViolations(err, "items["+strconv.Itoa(i)+"]."); len(violations) > 0 {
		return serviceregistry.NewInvalidFields(violations, err)
	}
	return clientSafeBindError(err)
}

// normalizeRequest restores req to the zero-value instance when a JSON null
// body left it nil — indistinguishable from an empty body for the service —
// and compacts nil slice elements away.
func (a *action[M, REQ, RSP]) normalizeRequest(req *REQ) {
	if a.reqKind == reflect.Pointer && reflect.ValueOf(*req).IsNil() {
		*req = a.newRequest()
	}
	compactNilSliceElements(reflect.ValueOf(req))
}

// normalizeModel is the model-path counterpart of normalizeRequest for
// handlers that bind the request body straight into the model type. Model
// types are pointers by construction, so only the nil restore and the slice
// compaction apply.
func (a *action[M, REQ, RSP]) normalizeModel(m *M) {
	if reflect.ValueOf(*m).IsNil() {
		*m = a.newModel()
	}
	compactNilSliceElements(reflect.ValueOf(m))
}

// normalizeBatch compacts nil entries out of the bound batch payload
// so the shared batch pipeline never dereferences a nil item.
func normalizeBatch[M types.Model](req *batch[M]) {
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
