package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
)

// TestModelRegistryAPIDocIsCurrent fails when the checked-in generated file
// stops matching what the generator produces. Generating at build time is what
// keeps the runtime from parsing Go sources, but it also means a forgotten
// `make generate` would ship stale descriptions; this is what catches that.
func TestModelRegistryAPIDocIsCurrent(t *testing.T) {
	t.Chdir("../../../..")

	want, err := buildModelRegistryAPIDoc()
	if err != nil {
		t.Fatalf("buildModelRegistryAPIDoc() error = %v", err)
	}

	path := filepath.Join(modelRegistryPkgDir, constants.FileAPIDocGen)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if string(got) != want {
		t.Fatalf("%s is out of date, run `make generate`", path)
	}
}
