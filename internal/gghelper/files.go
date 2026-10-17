package gghelper

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
)

// FileExists reports whether a file or directory exists at filename.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// EnsureParentDir creates the directory filename would be written into when
// it does not exist yet: model/sample for model/sample/record.go.
func EnsureParentDir(filename string) error {
	dir := filepath.Dir(filename)

	var err error
	if _, err = os.Stat(dir); err == nil {
		return nil
	} else if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	return err
}

// RelativePath returns filePath relative to the working directory, where gg
// runs, when possible, and filePath itself otherwise.
func RelativePath(filePath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		return filePath
	}
	return relPath
}

// PairedTestFiles returns the test files on disk that pair with the service
// file servicePath: its external form first, then its internal form, as in
// create_test.go and create_internal_test.go for create.go. They go with the
// service file: gg prune deletes them with a service file it deletes, and gg
// module copy with a stale one.
func PairedTestFiles(servicePath string) []string {
	stem := strings.TrimSuffix(servicePath, ".go")
	var tests []string
	for _, path := range []string{stem + ggconst.PatternTestFile, stem + "_internal" + ggconst.PatternTestFile} {
		if _, err := os.Stat(path); err == nil {
			tests = append(tests, path)
		}
	}
	return tests
}
