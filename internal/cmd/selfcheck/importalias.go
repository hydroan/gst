package main

import (
	"fmt"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// lintConfigFile is the golangci-lint configuration at the checked root,
// whose importas section lists the packages spelt by an alias wherever they
// are imported.
const lintConfigFile = ".golangci.yml"

// lintConfig is what the check reads of the golangci-lint configuration:
// whether importas runs and requires the aliases it lists, and the packages
// it lists with their aliases.
type lintConfig struct {
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Importas struct {
				NoUnaliased bool `yaml:"no-unaliased"`
				Alias       []struct {
					Pkg   string `yaml:"pkg"`
					Alias string `yaml:"alias"`
				} `yaml:"alias"`
			} `yaml:"importas"`
		} `yaml:"settings"`
	} `yaml:"linters"`
}

// checkImportAlias reports the import aliases nothing calls for. An import
// carries an alias in two cases alone. The package is listed in the importas
// section of the lint configuration, which then names it by that alias
// wherever it is imported: golangci-lint holds that spelling, and this check
// reads the list to leave the listed packages alone. Or the package's own
// name is the name another import of the same file binds. Nothing else
// calls for one: an identifier named like the package, at package level or
// inside a function, is renamed rather than the import aliased, and the name
// of the package the file belongs to is no clash, a file of package logger
// may import another package named logger and call it by that name. A
// package is spelt the same wherever a clash calls for an alias, so a second
// spelling of one package anywhere in the tree is reported too, once for the
// package. A name that is the package's own, spelt out because it differs
// from the last element of the import path, is no alias.
func checkImportAlias(root string, pkgs []*packages.Package) ([]violation, error) {
	listed, err := listedAliases(root)
	if err != nil {
		return nil, err
	}
	// The aliases each package is imported by, with where each spelling is
	// first seen, in file order.
	type spelling struct {
		alias string
		file  string
		line  int
	}
	spellings := make(map[string][]spelling)
	var found []violation
	for _, p := range checkedPackages(pkgs) {
		for _, file := range p.Syntax {
			path := relative(root, p.Fset.Position(file.Pos()).Filename)
			if filepath.IsAbs(path) {
				continue // a file the build generated outside the tree
			}
			for _, spec := range file.Imports {
				if spec.Name == nil || spec.Name.Name == "_" || spec.Name.Name == "." {
					continue
				}
				importPath, _ := strconv.Unquote(spec.Path.Value)
				imported, ok := p.Imports[importPath]
				if !ok {
					continue
				}
				alias, name := spec.Name.Name, imported.Name
				if alias == name || slices.ContainsFunc(listed, func(re *regexp.Regexp) bool { return re.MatchString(importPath) }) {
					continue
				}
				line := p.Fset.Position(spec.Pos()).Line
				spellings[importPath] = append(spellings[importPath], spelling{alias: alias, file: path, line: line})
				if importClashes(p, file, spec, name) {
					continue
				}
				found = append(found, violation{
					File:    path,
					Message: fmt.Sprintf("Import '%s' at %s:%d is aliased %s though no other import of the file is named %s: import it under its own name", importPath, path, line, alias, name),
				})
			}
		}
	}
	for importPath, seen := range spellings {
		sort.Slice(seen, func(i, j int) bool {
			if seen[i].file != seen[j].file {
				return seen[i].file < seen[j].file
			}
			return seen[i].line < seen[j].line
		})
		first := make(map[string]spelling)
		for _, s := range seen {
			if _, ok := first[s.alias]; !ok {
				first[s.alias] = s
			}
		}
		if len(first) < 2 {
			continue
		}
		var parts []string
		for _, alias := range slices.Sorted(maps.Keys(first)) {
			parts = append(parts, fmt.Sprintf("%s (%s:%d)", alias, first[alias].file, first[alias].line))
		}
		found = append(found, violation{
			File:    seen[0].file,
			Message: fmt.Sprintf("Package '%s' is aliased %s: use one alias for it", importPath, strings.Join(parts, " and ")),
		})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].File != found[j].File {
			return found[i].File < found[j].File
		}
		return found[i].Message < found[j].Message
	})
	return found, nil
}

// listedAliases returns the packages the importas section of the lint
// configuration at root lists, as the patterns importas matches import
// paths against, anchored the way it anchors them. The configuration must
// enable importas and require its aliases (no-unaliased), or the listed
// spelling would hold nowhere.
func listedAliases(root string) ([]*regexp.Regexp, error) {
	raw, err := os.ReadFile(filepath.Join(root, lintConfigFile))
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", lintConfigFile)
	}
	var cfg lintConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, errors.Wrapf(err, "parse %s", lintConfigFile)
	}
	if !slices.Contains(cfg.Linters.Enable, "importas") {
		return nil, errors.Newf("importas is not enabled in %s: the check reads the aliases it lists", lintConfigFile)
	}
	if !cfg.Linters.Settings.Importas.NoUnaliased {
		return nil, errors.Newf("%s does not set importas no-unaliased: a listed alias must hold wherever its package is imported", lintConfigFile)
	}
	listed := make([]*regexp.Regexp, 0, len(cfg.Linters.Settings.Importas.Alias))
	for _, entry := range cfg.Linters.Settings.Importas.Alias {
		re, err := regexp.Compile("^" + entry.Pkg + "$")
		if err != nil {
			return nil, errors.Wrapf(err, "importas alias %q in %s", entry.Pkg, lintConfigFile)
		}
		listed = append(listed, re)
	}
	return listed, nil
}

// checkedPackages returns the packages whose files the check reads, each
// file once: a package compiled with its internal test files stands for the
// package, since it holds every file and every package-level name; an
// external test package stands for itself; the test main go/packages
// synthesizes holds no file of the tree.
func checkedPackages(pkgs []*packages.Package) []*packages.Package {
	byPath := make(map[string]*packages.Package)
	for _, p := range pkgs {
		if strings.HasSuffix(p.PkgPath, ".test") {
			continue
		}
		if cur, ok := byPath[p.PkgPath]; !ok || (isInternalTestVariant(p) && !isInternalTestVariant(cur)) {
			byPath[p.PkgPath] = p
		}
	}
	chosen := make([]*packages.Package, 0, len(byPath))
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		chosen = append(chosen, byPath[path])
	}
	return chosen
}

// importClashes reports whether name, the own name of the package the
// import spec of file brings in, is the name another import of the file
// binds: its alias, or the own name of the package it brings in.
func importClashes(p *packages.Package, file *ast.File, spec *ast.ImportSpec, name string) bool {
	for _, other := range file.Imports {
		if other == spec {
			continue
		}
		bound := ""
		if other.Name != nil {
			bound = other.Name.Name
		} else if otherPath, err := strconv.Unquote(other.Path.Value); err == nil {
			if imported, ok := p.Imports[otherPath]; ok {
				bound = imported.Name
			}
		}
		if bound == name {
			return true
		}
	}
	return false
}
