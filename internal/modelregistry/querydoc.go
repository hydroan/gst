package modelregistry

import (
	_ "embed"
	"reflect"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/structdoc"
)

// querySource embeds the query parameter source code so its field doc comments
// stay available to the OpenAPI generator in binaries that ship without Go
// source files, eg. container images built from the compiled binary only.
//
//go:embed query.go
var querySource []byte

// queryDocTypes lists the structs of query.go that carry query tags, in
// declaration order. Every one of them reaches the generated document as a set
// of query parameters, so each needs its field comments registered.
var queryDocTypes = []string{"Query", "Pagination", "Cursor"}

func init() {
	docs, err := structdoc.ParseSource("query.go", querySource)
	if err != nil {
		// The embedded source always parses; guard against future edits.
		return
	}
	pkgPath := reflect.TypeFor[Query]().PkgPath()
	for _, typeName := range queryDocTypes {
		if doc, ok := docs[typeName]; ok {
			apidoc.Register(pkgPath, typeName, doc)
		}
	}
}
