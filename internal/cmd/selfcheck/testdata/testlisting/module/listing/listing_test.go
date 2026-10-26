package listing

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestListsInside lists the package's own directory, its testdata and
// directories the test made, which the check leaves alone.
func TestListsInside(t *testing.T) {
	_, _ = os.ReadDir(".")
	_, _ = os.ReadDir("testdata")
	_, _ = os.ReadDir(t.TempDir())
	made, _ := os.MkdirTemp("", "listing")
	_, _ = os.ReadDir(made)
	_, _ = os.ReadDir(ownDir())
	_, _ = filepath.Glob(filepath.Join(ownDir(), "testdata", "*.txt"))
	for _, dir := range []string{"testdata/a", "testdata/b"} {
		_, _ = os.ReadDir(dir)
	}
	listInside(t, "testdata")
	listInside(t, filepath.Join("testdata", "a"))
}

// TestListsElsewhere lists directories outside the package, or ones the
// check cannot follow back, which it reports.
func TestListsElsewhere(t *testing.T) {
	_, _ = os.ReadDir("..")
	_ = filepath.WalkDir(filepath.Join("..", "other"), func(string, fs.DirEntry, error) error { return nil })
	_, _ = os.ReadDir(os.Getenv("ROOT"))
	_, _ = filepath.Glob("/etc/*")
	_, _ = os.ReadDir(filepath.Dir(ownDir()))
	_ = fs.WalkDir(os.DirFS(".."), ".", func(string, fs.DirEntry, error) error { return nil })
	listAnywhere(t, "testdata")
	listAnywhere(t, "../other")
}

// listInside lists dir, which every caller names inside the package.
func listInside(t *testing.T, dir string) {
	t.Helper()
	_, _ = os.ReadDir(dir)
}

// listAnywhere lists dir, which one caller names outside the package.
func listAnywhere(t *testing.T, dir string) {
	t.Helper()
	_, _ = os.ReadDir(dir)
}

// ownDir returns the directory of the package, through the test file
// runtime.Caller reports.
func ownDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}
