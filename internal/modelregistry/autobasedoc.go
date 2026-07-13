package modelregistry

import (
	_ "embed"
	"reflect"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/structdoc"
)

// autoBaseSource embeds the AutoBase source code so its field doc comments
// stay available to the OpenAPI generator in binaries that ship without Go
// source files, eg. container images built from the compiled binary only.
//
//go:embed autobase.go
var autoBaseSource []byte

func init() {
	docs, err := structdoc.ParseSource("autobase.go", autoBaseSource)
	if err != nil {
		// The embedded source always parses; guard against future edits.
		return
	}
	if doc, ok := docs["AutoBase"]; ok {
		apidoc.Register(reflect.TypeFor[AutoBase]().PkgPath(), "AutoBase", doc)
	}
}
