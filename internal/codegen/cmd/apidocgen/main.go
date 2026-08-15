// Command apidocgen regenerates the apidoc registration file of the framework
// package that ships struct doc comments to the OpenAPI generator.
//
// Run it from the repository root through `make generate`. Registering at build
// time is what keeps the runtime from parsing Go sources: a deployed binary
// documents exactly what a development machine does. A forgotten run is caught
// by TestModelRegistryAPIDocIsCurrent, not by review.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
)

const (
	// modelRegistryPkgDir is the framework package declaring the model structs
	// an application embeds: the base models and the query parameter structs.
	// Their doc comments become schema and query parameter descriptions, so
	// they reach the generator through the apidoc registry, exactly like an
	// application's own models reach it through model/apidoc.gen.go.
	modelRegistryPkgDir = "internal/modelregistry"

	// modelRegistryPkgName is the Go package name declared in modelRegistryPkgDir.
	modelRegistryPkgName = "modelregistry"
)

func main() {
	code, err := buildModelRegistryAPIDoc()
	if err != nil {
		fmt.Fprintln(os.Stderr, "apidocgen:", err)
		os.Exit(1)
	}

	path := filepath.Join(modelRegistryPkgDir, constants.FileAPIDocGen)
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil { //nolint:gosec // generated source, world readable on purpose
		fmt.Fprintln(os.Stderr, "apidocgen:", err)
		os.Exit(1)
	}
	fmt.Println("generated", path)
}

// buildModelRegistryAPIDoc renders the apidoc registration file for
// modelRegistryPkgDir, registering every struct the package declares rather
// than a list of type names, so a struct added later is documented without a
// second edit.
//
// The working directory must be the repository root. Generating and checking
// both go through this function, so the two can never disagree about what the
// file should contain.
func buildModelRegistryAPIDoc() (string, error) {
	entries, err := codegen.ExtractAPIDocs(constants.ImportPathGst, modelRegistryPkgDir, []string{constants.FileAPIDocGen})
	if err != nil {
		return "", errors.Wrapf(err, "extract %s api docs", modelRegistryPkgDir)
	}

	code, err := gen.BuildAPIDocFile(modelRegistryPkgName, entries)
	if err != nil {
		return "", errors.Wrapf(err, "build %s api doc file", modelRegistryPkgDir)
	}
	return code, nil
}
