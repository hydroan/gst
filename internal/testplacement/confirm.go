package testplacement

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// confirmExternal type-checks the candidates of p rewritten as external tests
// and returns the ones that compile that way, so a candidate the static pass
// misjudged is never reported. The candidates move together first, since they
// may use each other; only when that fails is each one tried on its own.
func confirmExternal(root string, p *packages.Package, found []*testFile) ([]*testFile, error) {
	if len(found) == 0 {
		return nil, nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].path < found[j].path })

	ok, err := compilesExternally(root, p, found)
	if err != nil {
		return nil, err
	}
	if ok {
		return found, nil
	}
	var confirmed []*testFile
	for _, f := range found {
		ok, err := compilesExternally(root, p, []*testFile{f})
		if err != nil {
			return nil, err
		}
		if ok {
			confirmed = append(confirmed, f)
		}
	}
	return confirmed, nil
}

// compilesExternally reports whether p and its tests still type-check once
// files declare the external test package instead.
func compilesExternally(root string, p *packages.Package, files []*testFile) (bool, error) {
	overlay := make(map[string][]byte, len(files))
	for _, f := range files {
		src, err := os.ReadFile(f.path)
		if err != nil {
			return false, errors.Wrapf(err, "testplacement: read %s", f.path)
		}
		overlay[f.path] = asExternal(src, f, p.Name, p.PkgPath)
	}
	pattern := "."
	if dir := relative(root, filepath.Dir(files[0].path)); dir != "." {
		pattern = "./" + dir
	}
	pkgs, err := load(root, overlay, pattern)
	if err != nil {
		return false, err
	}
	for _, q := range pkgs {
		if len(q.Errors) > 0 {
			return false, nil
		}
	}
	return true, nil
}

// asExternal rewrites the source of f as a file of the external test package
// of the package name at path: the package clause gains _test, the package
// is imported under its name on the same line, keeping every other line where
// it was, and each identifier naming the package's own declarations is
// qualified with it.
func asExternal(src []byte, f *testFile, name, path string) []byte {
	type insertion struct {
		offset int
		text   string
	}
	clause := "_test"
	if len(f.qualify) > 0 {
		clause += fmt.Sprintf("; import %s %q", name, path)
	}
	insertions := []insertion{{offset: f.nameEnd, text: clause}}
	for _, offset := range f.qualify {
		insertions = append(insertions, insertion{offset: offset, text: name + "."})
	}
	// Inserting from the end keeps the offsets still to be used valid.
	sort.Slice(insertions, func(i, j int) bool { return insertions[i].offset > insertions[j].offset })

	out := append([]byte(nil), src...)
	for _, in := range insertions {
		out = append(out[:in.offset], append([]byte(in.text), out[in.offset:]...)...)
	}
	return out
}
