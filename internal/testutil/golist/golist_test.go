package golist_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hydroan/gst/internal/testutil/golist"
	"github.com/stretchr/testify/require"
)

// TestPackagesReportsThePackagesAPatternMatches pins what a pattern brings:
// one naming a package brings that package, and one ending in /... every
// package below, directory and import path patterns alike, each package with
// its Go files, those its build constraints leave out among them and its
// test files left out.
func TestPackagesReportsThePackagesAPatternMatches(t *testing.T) {
	root := writeSampleModule(t)
	record := filepath.Join(root, "record")
	item := filepath.Join(record, "item")

	pkgs := golist.Packages(t, "./record")
	require.Len(t, pkgs, 1)
	require.Equal(t, "example.com/sample/record", pkgs[0].ImportPath)
	require.Equal(t, record, pkgs[0].Dir)
	require.Equal(t, []string{"ignored.go", "record.go"}, pkgs[0].GoFiles(t))

	for _, pattern := range []string{"./record/...", "example.com/sample/record/..."} {
		pkgs = golist.Packages(t, pattern)
		require.Len(t, pkgs, 2, pattern)
		require.Equal(t, "example.com/sample/record", pkgs[0].ImportPath, pattern)
		require.Equal(t, "example.com/sample/record/item", pkgs[1].ImportPath, pattern)
		require.Equal(t, item, pkgs[1].Dir, pattern)
		require.Equal(t, []string{"item.go"}, pkgs[1].GoFiles(t), pattern)
	}
}

// TestPackagesFailsForAPatternWithoutAPackage pins that a pattern matching
// no package fails the test instead of bringing none, a missing directory, a
// package the module lacks and a /... with no package below alike, and so
// does a pattern holding ... anywhere but at its end.
func TestPackagesFailsForAPatternWithoutAPackage(t *testing.T) {
	writeSampleModule(t)

	for _, test := range []struct{ pattern, want string }{
		{"./missing/...", "go list reports no directory for ./missing/...: "},
		{"example.com/sample/missing", "go list reports no directory for example.com/sample/missing: "},
		{"example.com/sample/record/item/...", ""},
		{"example.com/sample/missing/...", "go list matches no package for example.com/sample/missing/..."},
		{"./.../item", "golist takes a pattern naming a package or ending in /..., not ./.../item"},
	} {
		message := fatalMessage(t, func(tb testing.TB) {
			tb.Helper()
			golist.Packages(tb, test.pattern)
		})
		if test.want == "" {
			require.Empty(t, message, test.pattern)
			continue
		}
		require.Contains(t, message, test.want, test.pattern)
	}
}

// writeSampleModule writes the module example.com/sample into a fresh
// directory and makes it the working directory, returning its path with no
// symbolic link in it, the way go list reports directories: record holds
// record.go, ignored.go, which its build constraints leave out, and two test
// files, one of them left out as well; record/item holds item.go.
func writeSampleModule(t *testing.T) string {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for name, source := range map[string]string{
		"go.mod":                 "module example.com/sample\n",
		"record/record.go":       "package record\n",
		"record/ignored.go":      "//go:build ignore\n\npackage record\n",
		"record/record_test.go":  "package record\n",
		"record/ignored_test.go": "//go:build ignore\n\npackage record\n",
		"record/item/item.go":    "package item\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	}
	t.Setenv("GOWORK", "off")
	t.Chdir(root)
	return root
}

// fatalMessage runs fn with a testing.TB whose Fatal and Fatalf end fn the way
// they end a test, and returns the message they were given; empty when fn
// returned on its own.
func fatalMessage(t *testing.T, fn func(tb testing.TB)) string {
	t.Helper()

	recorder := &fatalRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(recorder)
	}()
	<-done
	return recorder.message
}

// fatalRecorder is a testing.TB whose Fatal and Fatalf keep the message and
// end the goroutine calling them, as they end the goroutine of a test.
type fatalRecorder struct {
	testing.TB
	message string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatal(args ...any) {
	r.message = fmt.Sprint(args...)
	runtime.Goexit()
}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}
