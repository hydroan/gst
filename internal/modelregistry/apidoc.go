package modelregistry

import (
	_ "embed"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/structdoc"
)

// The sources declaring framework model structs are embedded so their doc
// comments stay available to the OpenAPI generator in binaries that ship
// without Go source files, eg. container images built from the compiled
// binary only.
var (
	//go:embed base.go
	baseSource []byte

	//go:embed autobase.go
	autoBaseSource []byte

	//go:embed query.go
	querySource []byte
)

func init() {
	registerEmbeddedDocs("base.go", baseSource)
	registerEmbeddedDocs("autobase.go", autoBaseSource)
	registerEmbeddedDocs("query.go", querySource)
}

// registerEmbeddedDocs registers the doc comments of every struct the embedded
// source file declares. Registering whatever the file declares, rather than a
// list of type names kept alongside it, is what keeps a struct added to an
// already embedded file documented without a second edit.
//
// The source is compiled into the binary from this very package, so a parse
// failure means the package itself stopped parsing; panicking says so at
// startup instead of serving an OpenAPI document whose descriptions silently
// went empty.
func registerEmbeddedDocs(filename string, source []byte) {
	docs, err := structdoc.ParseSource(filename, source)
	if err != nil {
		panic(errors.Wrapf(err, "modelregistry: parse embedded %s", filename))
	}

	pkgPath := reflect.TypeFor[Base]().PkgPath()
	for typeName, doc := range docs {
		apidoc.Register(pkgPath, typeName, doc)
	}
}
