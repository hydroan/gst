package openapigen

import (
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/structdoc"
)

// structDocCache caches the source-parsed doc of each type, keyed by package
// path and type name. Locating the source file goes through go/build, which
// shells out to `go list` in module mode, so a repeat lookup costs tens of
// milliseconds and a fork.
//
// Misses are cached too, and that is the case worth caching: a deployed binary
// ships no source files, so every lookup fails, and without the negative entry
// the failed lookup would repeat for every field of every documented model.
var (
	structDocCache = make(map[string]apidoc.StructDoc)
	structDocMutex sync.RWMutex
)

// cachedStructDocFromSource parses the source doc of a type at most once,
// serving both the field docs and the struct comment from the same entry so an
// unregistered type costs one source lookup rather than one per consumer.
func cachedStructDocFromSource(pkgPath, typeName string) apidoc.StructDoc {
	cacheKey := pkgPath + "." + typeName

	structDocMutex.RLock()
	cached, ok := structDocCache[cacheKey]
	structDocMutex.RUnlock()
	if ok {
		return cached
	}

	doc, _ := parseStructDocFromSource(pkgPath, typeName)

	structDocMutex.Lock()
	structDocCache[cacheKey] = doc
	structDocMutex.Unlock()

	return doc
}

// parseModelDocs returns a map with the doc comment for each field of the
// given model. The key is the struct field name, the value is the field doc
// comment.
//
// The apidoc registry is consulted first: gg-generated code and embedded
// framework sources register comments there at build time, which keeps the
// documentation available in binaries deployed without Go source files.
// When the struct is not registered, it falls back to locating and parsing
// the source file on disk (works in development environments), through the
// cache so each type is looked up at most once.
func parseModelDocs(t any) map[string]string {
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

	if doc, ok := apidoc.Lookup(pkgPath, typeName); ok {
		if doc.Fields == nil {
			return map[string]string{}
		}
		return doc.Fields
	}

	doc := cachedStructDocFromSource(pkgPath, typeName)
	if doc.Fields == nil {
		return map[string]string{}
	}
	return doc.Fields
}

// parseStructComment returns the doc comment of the struct itself (not its
// fields). Like parseModelDocs it prefers the apidoc registry and falls back
// to the source file through the shared cache, so asking for a type's comment
// and its field docs costs one source lookup between them.
func parseStructComment(t any) string {
	pkgPath, typeName := typeIdentity(t)
	if pkgPath == "" || typeName == "" {
		// Silently handle invalid type cases
		return ""
	}

	if doc, ok := apidoc.Lookup(pkgPath, typeName); ok {
		return doc.Comment
	}

	return cachedStructDocFromSource(pkgPath, typeName).Comment
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

// parseStructDocFromSource locates the source file declaring the type and
// parses its struct and field doc comments. It only works when Go source
// files are available on disk, which is the case in development but usually
// not in deployed binaries.
func parseStructDocFromSource(pkgPath, typeName string) (apidoc.StructDoc, bool) {
	sourceFile := findSourceFile(pkgPath, typeName)
	if sourceFile == "" {
		// Silently handle cases where source file is not found to avoid printing too many error messages
		return apidoc.StructDoc{}, false
	}

	docs, err := structdoc.ParseFile(sourceFile)
	if err != nil {
		return apidoc.StructDoc{}, false
	}

	doc, ok := docs[typeName]
	return doc, ok
}

// findSourceFile finds the source file based on package path and type name
func findSourceFile(pkgPath, typeName string) string {
	// First try to find using go/build package
	pkg, err := build.Import(pkgPath, ".", build.FindOnly)
	if err == nil && pkg.Dir != "" {
		// Search for .go files containing the target type in the package directory
		files, err := filepath.Glob(filepath.Join(pkg.Dir, "*.go"))
		if err == nil {
			// First check non-test files
			for _, file := range files {
				if strings.HasSuffix(file, "_test.go") {
					continue
				}

				// Check if the file contains the target type
				if containsType(file, typeName) {
					return file
				}
			}

			// If not found in non-test files, then check test files
			for _, file := range files {
				if !strings.HasSuffix(file, "_test.go") {
					continue
				}

				// Check if the file contains the target type
				if containsType(file, typeName) {
					return file
				}
			}
		}
	}

	// If go/build fails, try using call stack information
	for i := range 15 {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Check if the file contains the target package path
		if strings.Contains(file, strings.ReplaceAll(pkgPath, "/", string(filepath.Separator))) {
			return file
		}
	}

	return ""
}

// containsType checks if the file contains the specified type definition
func containsType(filename, typeName string) bool {
	content, err := os.ReadFile(filename)
	if err != nil {
		return false
	}

	// Simple string matching to check if it contains type definition
	// This is not a perfect solution, but it works for most cases
	typeDecl := "type " + typeName + " struct"
	return strings.Contains(string(content), typeDecl)
}
