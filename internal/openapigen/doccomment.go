package openapigen

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// openAPIDocComment removes the Go doc subject from API-facing text while
// preserving comments that do not begin with the exact declared symbol.
func openAPIDocComment(symbol, comment string) string {
	comment = strings.TrimSpace(comment)
	if symbol == "" || comment == "" {
		return comment
	}

	rest, ok := strings.CutPrefix(comment, symbol)
	if !ok || rest == "" {
		return comment
	}

	switch {
	case strings.HasPrefix(rest, ":"):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	case strings.HasPrefix(rest, "："):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "："))
	default:
		boundary, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsSpace(boundary) {
			return comment
		}
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		switch {
		case strings.HasPrefix(rest, ":"):
			rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		case strings.HasPrefix(rest, "："):
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "："))
		}
	}

	if rest == "" {
		return comment
	}
	switch {
	case strings.HasPrefix(rest, "是否"):
	case strings.HasPrefix(rest, "是"):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "是"))
	default:
		if trimmed, found := trimOpenAPIDocCopula(rest, "is"); found {
			rest = trimmed
		} else if trimmed, found := trimOpenAPIDocCopula(rest, "are"); found {
			rest = trimmed
		}
	}

	if rest == "" {
		return comment
	}
	first, size := utf8.DecodeRuneInString(rest)
	if upper := unicode.ToUpper(first); upper != first {
		rest = string(upper) + rest[size:]
	}
	return rest
}

func trimOpenAPIDocCopula(comment, copula string) (string, bool) {
	rest, ok := strings.CutPrefix(comment, copula)
	if !ok || rest == "" {
		return comment, false
	}
	boundary, _ := utf8.DecodeRuneInString(rest)
	if !unicode.IsSpace(boundary) {
		return comment, false
	}
	return strings.TrimLeftFunc(rest, unicode.IsSpace), true
}

// openAPIStructComment returns the API-facing doc comment of a model type.
func openAPIStructComment(typ reflect.Type) string {
	instance := elemInstance(typ)
	_, typeName := typeIdentity(instance)
	return openAPIDocComment(typeName, parseStructComment(instance))
}

// elemInstance creates a model instance for comment parsing, unwrapping
// slice types to their element type.
func elemInstance(typ reflect.Type) any {
	if typ.Kind() == reflect.Slice {
		return reflect.New(typ.Elem()).Interface()
	}
	return reflect.New(typ).Interface()
}
