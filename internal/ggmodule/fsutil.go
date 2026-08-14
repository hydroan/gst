package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/codegen/constants"
)

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func goFilesInDir(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipModuleSourceDir(root, path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isGoSourceFile(info.Name()) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	return files, err
}

// skipModuleSourceDir reports whether a directory reached while walking a module
// source tree must not be descended into. vendor and testdata hold files that
// are not part of the module's own source, so model discovery and file copy must
// agree on skipping them; consuming this single decision keeps the two walks
// from disagreeing about what counts as module code.
func skipModuleSourceDir(root string, path string, name string) bool {
	return path != root && (name == constants.DirVendor || name == constants.DirTestData)
}

func goFilesInPackageDir(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !isGoSourceFile(entry.Name()) {
			continue
		}
		files = append(files, filepath.Join(root, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func isGoSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasPrefix(name, ".")
}

// generatedFileHeader matches the Go convention for generated files
// (https://go.dev/s/generatedcode). gst's own consts.CodeGeneratedComment()
// header matches it, and so does the output of other generators such as
// mockgen or stringer.
var generatedFileHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// isGeneratedFile reports whether the file marks itself as generated code
// under the Go convention: a matching comment line before the package clause.
func isGeneratedFile(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if generatedFileHeader.MatchString(line) {
			return true, nil
		}
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			break
		}
	}
	return false, nil
}

func ensureParentDir(filename string) error {
	dir := filepath.Dir(filename)

	var err error
	if _, err = os.Stat(dir); err == nil {
		return nil
	} else if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	return err
}

// requirePathUnderRoot returns path cleaned and verified to be under root (no path traversal).
func requirePathUnderRoot(path, root string) (string, error) {
	path = filepath.Clean(path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s is not under root %s", path, root)
	}
	return path, nil
}

func canonicalModuleCopyPath(baseDir string, path string) (string, error) {
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return realPath, nil
	}
	return abs, nil
}
