package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
)

// TestAPIDocsAreCurrent fails when a checked-in generated file stops matching
// what the generator produces. Generating at build time is what keeps the
// runtime from parsing Go sources, but it also means a forgotten
// `make generate` would ship stale descriptions; this is what catches that.
func TestAPIDocsAreCurrent(t *testing.T) {
	t.Chdir("../../..")

	for _, pkg := range documentedPackages {
		want, err := buildAPIDoc(pkg)
		if err != nil {
			t.Fatalf("buildAPIDoc(%s) error = %v", pkg.dir, err)
		}

		path := filepath.Join(pkg.dir, ggconst.FileAPIDocGen)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		if string(got) != want {
			t.Fatalf("%s is out of date, run `make generate`", path)
		}
	}
}
