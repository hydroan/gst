// Package golist reads the packages of the module a test runs in through go
// list, for a test that reads the source of packages other than its own.
//
// go test keys a cached result on what the test read: every file it opened,
// every path it stat'ed and every entry of every directory it listed, by
// name, size and modification time. A test listing another package's
// directory, or the repository's, reruns whenever anything there changes, a
// test file of that package saved or git writing .git, and its result is
// never reused. go list runs in a child process, whose reads go test does not
// record: from it a test learns the packages a pattern matches and their
// files without listing a directory, and opens only the files it reads.
//
// What the child read, the package stats in the test, so that a cached result
// is dropped once it no longer holds. Packages stats the directory each
// pattern starts from, whose modification time moves when a package is added
// right below it or removed; GoFiles stats the directory of the package whose
// files the test asks for and those above it up to that start, which notices
// a file of the package, or a package below, added or removed. A directory
// changes its modification time whenever a file in it is saved through a
// temporary file, so a package the test passes over stays out of what it
// read.
package golist

import (
	"bytes"
	"encoding/json"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Package is one package a pattern of Packages matches.
type Package struct {
	// ImportPath is the package's import path.
	ImportPath string
	// Dir is the absolute path of the package's directory.
	Dir string

	// goFiles are the names of the package's Go files (see GoFiles).
	goFiles []string
	// base is the directory the pattern matching the package starts from:
	// the package's own for a pattern naming it, the directory of what
	// precedes /... for a pattern ending in it.
	base string
}

// Packages returns the packages the patterns match, in the order go list
// reports them, resolving each pattern the way go list does in the test's
// working directory: an import path or a directory, naming one package or,
// ending in /..., every package below. It stats the directory each pattern
// starts from. It fails the test for a pattern holding ... anywhere but at
// its end, and for one that matches no package, which would leave nothing to
// stat: a missing directory, a package outside the module, or a /... with no
// package below.
func Packages(tb testing.TB, patterns ...string) []Package {
	tb.Helper()
	for _, pattern := range patterns {
		if strings.Contains(strings.TrimSuffix(pattern, "/..."), "...") {
			tb.Fatalf("golist takes a pattern naming a package or ending in /..., not %s", pattern)
		}
	}
	// #nosec G204 -- the flags are literals and the patterns come from the
	// test, which names the packages it reads.
	list := exec.Command("go", append([]string{"list", "-e", "-json=ImportPath,Dir,Match,GoFiles,CgoFiles,IgnoredGoFiles,Error"}, patterns...)...)
	var stderr bytes.Buffer
	list.Stderr = &stderr
	output, err := list.Output()
	if err != nil {
		tb.Fatalf("go list %s: %v\n%s", strings.Join(patterns, " "), err, stderr.String())
	}

	var pkgs []Package
	bases := make(map[string]string, len(patterns))
	for decoder := json.NewDecoder(bytes.NewReader(output)); decoder.More(); {
		var listed struct {
			ImportPath     string   `json:"ImportPath"`
			Dir            string   `json:"Dir"`
			Match          []string `json:"Match"`
			GoFiles        []string `json:"GoFiles"`
			CgoFiles       []string `json:"CgoFiles"`
			IgnoredGoFiles []string `json:"IgnoredGoFiles"`
			Error          *struct {
				Err string `json:"Err"`
			} `json:"Error"`
		}
		if err = decoder.Decode(&listed); err != nil {
			tb.Fatalf("read what go list %s reports: %v", strings.Join(patterns, " "), err)
		}
		if listed.Dir == "" {
			reason := "no reason given"
			if listed.Error != nil {
				reason = listed.Error.Err
			}
			tb.Fatalf("go list reports no directory for %s: %s", listed.ImportPath, reason)
		}
		pkg := Package{ImportPath: listed.ImportPath, Dir: listed.Dir}
		for _, name := range slices.Concat(listed.GoFiles, listed.CgoFiles, listed.IgnoredGoFiles) {
			if !strings.HasSuffix(name, "_test.go") {
				pkg.goFiles = append(pkg.goFiles, name)
			}
		}
		slices.Sort(pkg.goFiles)
		for _, pattern := range listed.Match {
			base := patternBase(tb, pattern, listed.ImportPath, listed.Dir)
			bases[pattern] = base
			if len(base) > len(pkg.base) {
				pkg.base = base
			}
		}
		pkgs = append(pkgs, pkg)
	}
	for _, pattern := range patterns {
		base, ok := bases[pattern]
		if !ok {
			tb.Fatalf("go list matches no package for %s\n%s", pattern, stderr.String())
		}
		if _, err = stat(base); err != nil {
			tb.Fatal(err)
		}
	}
	return pkgs
}

// patternBase returns the directory pattern starts from, given a package it
// matches, at importPath in dir: dir for a pattern naming the package, and
// for one ending in /... the directory of what precedes it, which holds dir.
// A directory pattern resolves against the working directory, ./record/...
// in /repo giving /repo/record; an import path one against the package's,
// example.com/m/... matching example.com/m/record/item in
// /repo/record/item giving /repo.
func patternBase(tb testing.TB, pattern, importPath, dir string) string {
	tb.Helper()
	prefix, wildcard := strings.CutSuffix(pattern, "/...")
	switch {
	case !wildcard:
		return dir
	case build.IsLocalImport(prefix) || filepath.IsAbs(prefix):
		base, err := filepath.Abs(prefix)
		if err != nil {
			tb.Fatal(err)
		}
		return base
	default:
		return strings.TrimSuffix(dir, filepath.FromSlash(strings.TrimPrefix(importPath, prefix)))
	}
}

// GoFiles returns the names of the package's Go files, sorted: those its
// build constraints leave out among them, its test files left out. It stats
// the package's directory and those above it up to the one its pattern starts
// from.
func (p Package) GoFiles(tb testing.TB) []string {
	tb.Helper()
	for dir := p.Dir; ; dir = filepath.Dir(dir) {
		if _, err := stat(dir); err != nil {
			tb.Fatal(err)
		}
		if dir == p.base || dir == filepath.Dir(dir) {
			break
		}
	}
	return slices.Clone(p.goFiles)
}

// stat is os.Stat, which the package's tests replace to see what it stats.
var stat = os.Stat
