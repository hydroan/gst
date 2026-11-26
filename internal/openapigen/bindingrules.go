package openapigen

import (
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelschema"
)

// The binding rules the document reads, under the names the validator behind
// the binding tag knows them by.
const (
	ruleRequired = "required"
	ruleDive     = "dive"
	ruleKeys     = "keys"
	ruleEndKeys  = "endkeys"
	ruleMin      = "min"
	ruleMax      = "max"
	ruleLen      = "len"
	ruleGT       = "gt"
	ruleGTE      = "gte"
	ruleLT       = "lt"
	ruleLTE      = "lte"
	ruleOneOf    = "oneof"
)

// bindingRuleFormats maps the binding rules holding a string to a format to
// the name a schema states the format by.
var bindingRuleFormats = map[string]string{
	"email":            "email",
	"url":              "uri",
	"uri":              "uri",
	"uuid":             "uuid",
	"uuid3":            "uuid",
	"uuid4":            "uuid",
	"uuid5":            "uuid",
	"ipv4":             "ipv4",
	"ipv6":             "ipv6",
	"hostname":         "hostname",
	"hostname_rfc1123": "hostname",
}

// silentBindingRules are the rules a document has nothing to say of: the
// ones deciding when the others apply, and the framework's readonly, which
// the request bodies document by leaving the field out.
var silentBindingRules = []string{"", "-", "omitempty", "omitnil", "omitzero", modelschema.RuleReadOnly}

// oneOfValues splits the parameter of a oneof rule the way the validator
// does: at spaces, a value holding one written between single quotes.
var oneOfValues = regexp.MustCompile(`'[^']*'|\S+`)

// documentBindingRules writes into schema, the schema of a field of the Go
// type typ, what the binding rules of the field state in schema keywords,
// and reports whether it wrote anything: rules are the field's own and
// elementRules the ones after its dive (see modelschema.BindingRules and
// modelschema.ElementBindingRules), written to the schema of the items of a
// list or of the values of a map. The bounds of a string, a number, a list
// and a map become minLength and maxLength, minimum and maximum, minItems
// and maxItems, minProperties and maxProperties; oneof becomes enum, unless
// the type registered its values; a rule holding a string to a format (see
// bindingRuleFormats) becomes that format; and required holds a string to
// one character at least, the property being listed as required by the
// caller. A rule no keyword states is written into the description as
// declared, after what the description holds, the rules of the keys of a
// map and the ones after a second dive among them: for
//
//	Code string   `binding:"len=8,alphanum"`
//	Tags []string `binding:"max=5,dive,required,max=16"`
//
// Code gets minLength 8, maxLength 8 and the description "Validation:
// alphanum", Tags maxItems 5 and items of minLength 1 and maxLength 16.
func documentBindingRules(schema *openapi3.Schema, typ reflect.Type, rules, elementRules []string) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	wrote := false
	note := func(label string, unstated []string) {
		if len(unstated) == 0 {
			return
		}
		line := label + ": " + strings.Join(unstated, ", ")
		if schema.Description != "" {
			line = schema.Description + "\n\n" + line
		}
		schema.Description, wrote = line, true
	}

	var unstated []string
	for _, rule := range rules {
		switch {
		case slices.Contains(silentBindingRules, rule):
		case rule == ruleRequired && typ.Kind() != reflect.String:
			// The property is listed as required, which says it all.
		case stateBindingRule(schema, typ, rule):
			wrote = true
		default:
			unstated = append(unstated, rule)
		}
	}
	note("Validation", unstated)
	if len(elementRules) == 0 {
		return wrote
	}

	// The elements are described by a schema of their own, copied before it
	// is written to like the field's; one the document refers to by name is
	// shared with every field of the type and is left as it is.
	var element *openapi3.SchemaRef
	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		element = schema.Items
	case reflect.Map:
		element = schema.AdditionalProperties.Schema
	}
	var elementSchema *openapi3.Schema
	if element != nil && element.Ref == "" && element.Value != nil {
		copied := *element.Value
		elementSchema = &copied
	}
	var ofKeys, ofElements []string
	inKeys := false
rules:
	for i, rule := range elementRules {
		switch {
		case rule == ruleKeys:
			inKeys = true
		case rule == ruleEndKeys:
			inKeys = false
		case inKeys:
			ofKeys = append(ofKeys, rule)
		case rule == ruleDive:
			ofElements = append(ofElements, elementRules[i:]...)
			break rules
		case slices.Contains(silentBindingRules, rule):
		case elementSchema != nil && stateBindingRule(elementSchema, typ.Elem(), rule):
			wrote = true
		default:
			ofElements = append(ofElements, rule)
		}
	}
	if elementSchema != nil {
		written := &openapi3.SchemaRef{Value: elementSchema}
		if typ.Kind() == reflect.Map {
			schema.AdditionalProperties.Schema = written
		} else {
			schema.Items = written
		}
	}
	note("Validation of each key", ofKeys)
	note("Validation of each element", ofElements)
	return wrote
}

// stateBindingRule writes into schema, the schema of a value of the Go type
// typ, the keyword rule states of such a value, and reports whether a
// keyword states it. A value whose schema is not of the type its Go kind
// suggests, a byte slice documented as a string among them, has none.
func stateBindingRule(schema *openapi3.Schema, typ reflect.Type, rule string) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	name, param, _ := strings.Cut(rule, "=")
	is := func(schemaType string) bool { return schema.Type != nil && schema.Type.Is(schemaType) }
	switch kind := typ.Kind(); {
	case kind == reflect.String && is(openapi3.TypeString):
		switch format, formatted := bindingRuleFormats[name]; {
		case name == ruleRequired:
			schema.MinLength = max(schema.MinLength, 1)
		case name == ruleOneOf && len(schema.Enum) == 0:
			for _, value := range oneOfValues.FindAllString(param, -1) {
				schema.Enum = append(schema.Enum, strings.Trim(value, "'"))
			}
		case name == ruleOneOf:
		case formatted && param == "" && schema.Format == "":
			schema.Format = format
		default:
			return stateCount(name, param, &schema.MinLength, &schema.MaxLength)
		}
		return true
	case isNumberKind(kind) && (is(openapi3.TypeInteger) || is(openapi3.TypeNumber)):
		return stateNumberRule(schema, kind, name, param)
	case (kind == reflect.Slice || kind == reflect.Array) && is(openapi3.TypeArray):
		return stateCount(name, param, &schema.MinItems, &schema.MaxItems)
	case kind == reflect.Map && is(openapi3.TypeObject):
		return stateCount(name, param, &schema.MinProps, &schema.MaxProps)
	}
	return false
}

// stateCount writes the bound a rule sets on a count — the characters of a
// string, the items of a list, the properties of a map — into least and
// most, and reports whether the rule sets one: min, gte and gt set the
// least, max, lte and lt the most, and len both.
func stateCount(name, param string, least *uint64, most **uint64) bool {
	count, err := strconv.ParseUint(param, 10, 64)
	if err != nil {
		return false
	}
	switch name {
	case ruleMin, ruleGTE:
		*least = count
	case ruleGT:
		*least = count + 1
	case ruleMax, ruleLTE:
		*most = &count
	case ruleLT:
		if count == 0 {
			return false
		}
		count--
		*most = &count
	case ruleLen:
		*least, *most = count, &count
	default:
		return false
	}
	return true
}

// stateNumberRule writes the keyword a rule states of a number of kind into
// schema, and reports whether one states it: min and gte set the minimum,
// gt an exclusive one, max and lte the maximum, lt an exclusive one, and
// oneof the values, integers for an integer kind, unless the schema lists
// its values already.
func stateNumberRule(schema *openapi3.Schema, kind reflect.Kind, name, param string) bool {
	if name == ruleOneOf {
		values := make([]any, 0)
		for value := range strings.FieldsSeq(param) {
			var parsed any
			var err error
			if kind == reflect.Float32 || kind == reflect.Float64 {
				parsed, err = strconv.ParseFloat(value, 64)
			} else {
				parsed, err = strconv.ParseInt(value, 10, 64)
			}
			if err != nil {
				return false
			}
			values = append(values, parsed)
		}
		if len(schema.Enum) == 0 {
			schema.Enum = values
		}
		return true
	}

	bound, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return false
	}
	exclusive := true
	switch name {
	case ruleMin, ruleGTE:
		schema.Min = &bound
	case ruleGT:
		schema.Min, schema.ExclusiveMin = &bound, openapi3.ExclusiveBound{Bool: &exclusive}
	case ruleMax, ruleLTE:
		schema.Max = &bound
	case ruleLT:
		schema.Max, schema.ExclusiveMax = &bound, openapi3.ExclusiveBound{Bool: &exclusive}
	default:
		return false
	}
	return true
}

// isNumberKind reports whether kind is one of Go's integer or floating-point
// kinds.
func isNumberKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}
