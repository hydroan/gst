package openapigen

import (
	"reflect"

	"github.com/hydroan/gst/apidoc"
)

// modelFieldDocs returns a map with the doc comment for each field of the
// given model. The key is the struct field name, the value is the field doc
// comment.
//
// The apidoc registry is the only source: gg-generated code registers the
// project's model comments and the framework registers its own from embedded
// sources, both at build time. Nothing is read from disk, so a binary deployed
// without Go source files documents exactly what a development machine does.
func modelFieldDocs(t any) map[string]string {
	pkgPath, typeName := typeIdentity(t)
	if pkgPath == "" || typeName == "" {
		// An anonymous struct (a type alias to an unnamed struct) has neither a
		// package path nor a type name, so recover its field docs by matching
		// the struct's field signature against the registry.
		if fields, ok := anonStructFieldDocs(t); ok {
			return fields
		}
		return map[string]string{}
	}

	doc, ok := apidoc.Lookup(pkgPath, typeName)
	if !ok || doc.Fields == nil {
		return map[string]string{}
	}
	return doc.Fields
}

// modelStructComment returns the doc comment of the struct itself (not its
// fields), read from the same registry modelFieldDocs reads.
func modelStructComment(t any) string {
	pkgPath, typeName := typeIdentity(t)
	if pkgPath == "" || typeName == "" {
		// Silently handle invalid type cases
		return ""
	}

	doc, _ := apidoc.Lookup(pkgPath, typeName)
	return doc.Comment
}

// typeIdentity resolves the package path and type name of the given value,
// unwrapping pointers.
func typeIdentity(t any) (pkgPath, typeName string) {
	typ := reflect.TypeOf(t)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.PkgPath(), typ.Name()
}

// anonStructFieldDocs recovers the field docs of an anonymous struct value by
// matching its exported field names against the apidoc registry. It returns
// false when t is not a struct or no unambiguous signature match exists.
func anonStructFieldDocs(t any) (map[string]string, bool) {
	typ := reflect.TypeOf(t)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, false
	}

	names := exportedFieldNames(typ)
	if len(names) == 0 {
		return nil, false
	}
	return apidoc.LookupFieldsBySignature(names)
}

// exportedFieldNames returns the Go names of the exported, non-embedded fields
// of typ, matching the field set that structdoc records for a struct.
func exportedFieldNames(typ reflect.Type) []string {
	var names []string
	for field := range typ.Fields() {
		if field.Anonymous || !field.IsExported() {
			continue
		}
		names = append(names, field.Name)
	}
	return names
}
