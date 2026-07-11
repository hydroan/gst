package modelregistry

import (
	_ "embed"
	"reflect"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/structdoc"
)

// baseSource embeds the Base source code so its field doc comments stay
// available to the OpenAPI generator in binaries that ship without Go
// source files, eg. container images built from the compiled binary only.
//
//go:embed base.go
var baseSource []byte

func init() {
	docs, err := structdoc.ParseSource("base.go", baseSource)
	if err != nil {
		// The embedded source always parses; guard against future edits.
		return
	}
	if doc, ok := docs["Base"]; ok {
		apidoc.Register(reflect.TypeFor[Base]().PkgPath(), "Base", doc)
	}
}
