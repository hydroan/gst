// Command apidocgen regenerates the apidoc registration files of the framework
// packages that ship struct doc comments to the OpenAPI generator.
//
// Run it from the repository root through `make generate`. Registering at build
// time is what keeps the runtime from parsing Go sources: a deployed binary
// documents exactly what a development machine does. A forgotten run is caught
// by TestAPIDocsAreCurrent, not by review.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// documentedPackage is a framework package declaring structs an application
// embeds in its models: the directory it lives in and the Go package name it
// declares. Their doc comments become schema and query parameter
// descriptions, so they reach the generator through the apidoc registry,
// exactly like an application's own models reach it through
// model/apidoc.gen.go.
type documentedPackage struct {
	dir  string
	name string
}

// documentedPackages are the packages apidocgen registers the doc comments
// of: the base models and the query parameter structs of
// internal/modelregistry, and the tenant scope of internal/tenant.
var documentedPackages = []documentedPackage{
	{dir: "internal/modelregistry", name: "modelregistry"},
	{dir: "internal/tenant", name: "tenant"},
}

func main() {
	for _, pkg := range documentedPackages {
		code, err := buildAPIDoc(pkg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "apidocgen:", err)
			os.Exit(1)
		}

		path := filepath.Join(pkg.dir, ggconst.FileAPIDocGen)
		if err := os.WriteFile(path, []byte(code), 0o644); err != nil { //nolint:gosec // generated source, world readable on purpose
			fmt.Fprintln(os.Stderr, "apidocgen:", err)
			os.Exit(1)
		}
		fmt.Println("generated", path)
	}
}

// buildAPIDoc renders the apidoc registration file for pkg, registering every
// struct the package declares rather than a list of type names, so a struct
// added later is documented without a second edit.
//
// The working directory must be the repository root. Generating and checking
// both go through this function, so the two can never disagree about what the
// file should contain.
func buildAPIDoc(pkg documentedPackage) (string, error) {
	entries, err := modelinfo.ExtractAPIDocs(ggconst.ImportPathGst, pkg.dir, gghelper.NewProjectIgnore(), []string{ggconst.FileAPIDocGen})
	if err != nil {
		return "", errors.Wrapf(err, "extract %s api docs", pkg.dir)
	}

	code, err := gggen.BuildAPIDocFile(ggconst.ImportPathGst, pkg.name, entries)
	if err != nil {
		return "", errors.Wrapf(err, "build %s api doc file", pkg.dir)
	}
	return code, nil
}
