package controller

import (
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	entranslations "github.com/go-playground/validator/v10/translations/en"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
)

// This file holds the validation both transports run on what they decoded:
// the binding tags of a model or payload checked by gin's validator, whether
// an HTTP body or a request message carried the value, and the fields the
// validator refuses named for the client by the paths of field_path.go.

// validateRequest checks target against its binding tags with gin's
// validator: the check bindJSONRequest makes of a bound body and the call
// functions make of the value a request message decoded into, so a model or
// payload meets the same tags on both transports. The error is the
// validator's own, naming Go fields, with the type it checked (see
// refusedRequestError), for each transport to wrap in its client-safe
// message. A nil binding.Validator is gin's documented way to turn
// validation off; gin's own binding paths nil-check it, so this does the
// same.
func validateRequest(target any) error {
	if binding.Validator == nil {
		return nil
	}
	return withRequestType(binding.Validator.ValidateStruct(target), target)
}

// refusedRequestError is the error of a request the validator refused, with
// the type the validator checked: the type names the fields refused by the
// JSON path the client sent them under (see jsonFieldPath), which the
// validator's namespaces, one of tag names and one of Go names, do not tell
// on their own, an embedded struct being a level of both and none of the
// JSON. The validator's own error stays reachable through Unwrap.
type refusedRequestError struct {
	err error
	typ reflect.Type
}

// Error returns the validator's own message.
func (r *refusedRequestError) Error() string { return r.err.Error() }

// Unwrap returns the validator's own error.
func (r *refusedRequestError) Unwrap() error { return r.err }

// withRequestType returns err, the outcome of validating target, as the
// refusedRequestError of target when it is an error, and nil when it is nil.
func withRequestType(err error, target any) error {
	if err == nil {
		return nil
	}
	return &refusedRequestError{err: err, typ: reflect.TypeOf(target)}
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
// them under, renders each failure as the English sentence of its own
// translations, "name is a required field", and knows the rule declaring a
// field read-only (see modelschema.RuleReadOnly), which checks nothing: the
// write flows read the declaration, and the validator would refuse a rule it
// was never told of. A validator other than go-playground's is left as it
// is, its errors answered without naming a field (see clientSafeBindError).
func init() {
	if binding.Validator == nil {
		return
	}
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	engine.RegisterTagNameFunc(fieldJSONName)
	// The rule runs on a nil value as well, which the validator would refuse
	// under a rule it does not run; the registration fails for an empty rule
	// name or a nil function alone.
	_ = engine.RegisterValidation(modelschema.RuleReadOnly, func(validator.FieldLevel) bool { return true }, true)
	english := en.New()
	translator, _ := ut.New(english, english).GetTranslator(english.Locale())
	if err := entranslations.RegisterDefaultTranslations(engine, translator); err != nil {
		return
	}
	validatorEngine = engine
	validatorTranslator = translator
}

// fieldViolations returns the fields a validator error names, one violation
// per field: the field by its JSON key path relative to the request (see
// jsonFieldPath), with prefix in front, items[1]. for the item of a batch
// validated on its own, and a sentence for the failure. The sentence is the
// validator's own English sentence for the rule with the path in place of
// the bare field name it begins with, "address.city is a required field";
// for a rule the validator has no sentence for, one of its own or one a
// project registered, it is the path and the rule, "address.zip failed the
// hostname check", the parameter with the rule when it has one, "tag failed
// the startswith=ab check": the text the validator renders those with
// instead names the Go type of the request and is not the client's. nil for
// any other error, and for the errors of a validator other than the one
// init configured, whose sentences and field names are its own.
func fieldViolations(err error, prefix string) []types.FieldViolation {
	var request *refusedRequestError
	var fieldErrors validator.ValidationErrors
	if validatorEngine == nil || binding.Validator == nil || binding.Validator.Engine() != validatorEngine || !errors.As(err, &request) || !errors.As(request.err, &fieldErrors) {
		return nil
	}
	violations := make([]types.FieldViolation, 0, len(fieldErrors))
	for _, fe := range fieldErrors {
		path := prefix + jsonFieldPath(request.typ, fe)
		description := fe.Translate(validatorTranslator)
		if description == fe.Error() {
			rule := fe.Tag()
			if param := fe.Param(); param != "" {
				rule += "=" + param
			}
			description = path + " failed the " + rule + " check"
		} else {
			description = path + strings.TrimPrefix(description, fe.Field())
		}
		violations = append(violations, types.FieldViolation{Field: path, Description: description})
	}
	return violations
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
// see validateRequest. The error carries the type checked the way
// validateRequest's does (see withRequestType).
func validatePatchFields(target any, fields patchFieldSet) error {
	if binding.Validator == nil || len(fields) == 0 {
		return nil
	}
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return withRequestType(binding.Validator.ValidateStruct(target), target)
	}
	typ := reflect.TypeOf(target)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	prefix := typ.Name() + "."
	return withRequestType(engine.StructFiltered(target, func(ns []byte) bool {
		return !fields.covers(strings.TrimPrefix(string(ns), prefix))
	}), target)
}
