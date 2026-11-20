package controller

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// This file holds how a field is named to a client, the same whichever
// transport refused it: by the key its json tag gives it and, inside a
// request, by the path the contract spells, items[1].rank, an index in
// brackets and any other step behind a dot (see pathStep).

// jsonTagName returns the name a field's json tag gives it: the part before
// the comma, "-" for a field the tag leaves out, and "" for a field without
// a name in its tag, which encoding/json names after the field.
func jsonTagName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	return name
}

// fieldJSONName returns the name a field carries in JSON: the name its json
// tag gives it, and the field's own name when the tag gives none or leaves
// the field out. It names the fields for the validator (see init) and for
// the paths of the fields refused (see jsonFieldPath).
func fieldJSONName(field reflect.StructField) string {
	if name := jsonTagName(field); name != "" && name != "-" {
		return name
	}
	return field.Name
}

// jsonFieldPath returns the path of the field fe names, relative to the
// request, as the client sent it: the JSON name of each field down from
// typ, the type the validator checked, an index kept as it is, and nothing
// for an embedded struct without a name of its own, whose fields JSON
// promotes to its level; an embedded struct, or pointer to one, is what
// encoding/json promotes, an embedded value of any other type being a field
// under its type's name. The validator's namespace of Go names (see
// validator.FieldError.StructNamespace) drives the walk of the type; a
// namespace the walk cannot follow falls back to the validator's namespace
// of tag names (see fieldPath).
func jsonFieldPath(typ reflect.Type, fe validator.FieldError) string {
	fallback := fieldPath(fe.Namespace())
	var path []string
	current := typ
	for _, segment := range splitOutsideBrackets(fieldPath(fe.StructNamespace())) {
		name, indexes, indexed := strings.Cut(segment, "[")
		if indexed {
			indexes = "[" + indexes
		}
		for current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return fallback
		}
		var field reflect.StructField
		found := false
		for candidate := range current.Fields() {
			if candidate.Name == name {
				field, found = candidate, true
				break
			}
		}
		if !found {
			return fallback
		}
		embedded := field.Type
		for embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if field.Anonymous && jsonTagName(field) == "" && embedded.Kind() == reflect.Struct {
			if indexed {
				return fallback
			}
		} else {
			path = append(path, fieldJSONName(field)+indexes)
		}
		current = field.Type
		for range strings.Count(indexes, "[") {
			for current.Kind() == reflect.Pointer {
				current = current.Elem()
			}
			switch current.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map:
				current = current.Elem()
			default:
				return fallback
			}
		}
	}
	return strings.Join(path, ".")
}

// splitOutsideBrackets splits a namespace at the dots outside its brackets,
// the key of a map field carrying any character: items[a.b].name is the two
// segments items[a.b] and name.
func splitOutsideBrackets(namespace string) []string {
	var segments []string
	depth, start := 0, 0
	for i, r := range namespace {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				segments = append(segments, namespace[start:i])
				start = i + 1
			}
		}
	}
	return append(segments, namespace[start:])
}

// fieldPath returns the namespace of a validator error without the type of
// the request at its head: what follows the first dot outside the brackets
// a generic type's name may carry, batch[...].items[1].name being the
// namespace of an item's field. It is what the path of a field falls back
// to when jsonFieldPath cannot follow the type.
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

// Element returns the path of the element at index i of the repeated field
// at path, points[1]: what a generated conversion names a refused element by
// (see PathError), as the contract spells an index. The public grpc.Element
// forwards to it.
func Element(path string, i int) string {
	return pathStep(path, strconv.Itoa(i))
}

// Entry returns the path of the value under key of the map field at path,
// costs.k1, and by_rank[7] for a key of digits alone, which encoding/json
// spells like an index on the HTTP side: what a generated conversion names a
// refused map value by (see PathError). The public grpc.Entry forwards to it.
func Entry[K comparable](path string, key K) string {
	return pathStep(path, fmt.Sprint(key))
}

// itemPrefix is what the field of the item at index i of a batch is named
// behind, items[1]., in a refusal naming it.
func itemPrefix(i int) string {
	return Element("items", i) + "."
}

// pathStep spells step appended to path the way the contract spells a field
// path: a step of digits alone, an index or a numeric map key, in brackets,
// points[1], since encoding/json spells the two alike on the HTTP side (see
// indexedJSONPath) and the contract writes an index so; any other step
// behind a dot, meta.score; step alone when path is empty.
func pathStep(path, step string) string {
	if step != "" && strings.Trim(step, "0123456789") == "" {
		return path + "[" + step + "]"
	}
	if path == "" {
		return step
	}
	return path + "." + step
}
