package gghelper

import (
	"os"
	"path/filepath"
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
