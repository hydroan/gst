package gggen

import (
	"go/token"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// ResolveImportConflicts picks the alias each project import of a generated
// file needs. declared maps every import path to the name of the package it
// declares, and reserved lists the names the file's framework imports take. An
// import keeps its package name when no reserved name and no other import
// claims it, and spells the name out when the path does not tell it (see
// importSpecName), so the import shows the name the code refers to the package
// by; otherwise it is aliased with its last two path segments joined by an
// underscore ("svc/pkg1/user" becomes "pkg1_user"). Every import in a clash is
// aliased, not all but one: which import kept the name would depend on the
// others, and a new import sharing the name could take it away. An alias that
// still clashes with another name takes one more leading segment at a time, and
// one that runs out of segments gets a numeric suffix, so no two imports share
// a name and none takes a framework import's. Every alias is a Go identifier:
// characters an identifier cannot hold become underscores, and one that would
// start with a digit gets an underscore in front. The result maps each import
// path to the name it is written with, or to "" when it is written without one.
//
// For example, the service registration file reserves service and consts for
// its framework imports, so the imports
//
//	"helloworld/service/account/recorditem" // package recorditem
//	"helloworld/service/api/v2"             // package v2
//	"helloworld/service/sample/entry_item"  // package entryitem
//	"helloworld/service/sample/item"        // package item
//	"helloworld/service/sample/record_item" // package recorditem
//	"helloworld/service/sample/service"     // package service
//
// are written as
//
//	account_recorditem "helloworld/service/account/recorditem"
//	v2 "helloworld/service/api/v2"
//	entryitem "helloworld/service/sample/entry_item"
//	"helloworld/service/sample/item"
//	sample_record_item "helloworld/service/sample/record_item"
//	sample_service "helloworld/service/sample/service"
func ResolveImportConflicts(declared map[string]string, reserved ...string) map[string]string {
	paths := slices.Sorted(maps.Keys(declared))

	claims := make(map[string]int, len(paths)+len(reserved))
	for _, reservedName := range reserved {
		claims[reservedName]++
	}
	for _, importPath := range paths {
		claims[declared[importPath]]++
	}
	// depth is the number of trailing path segments an aliased import's name
	// is built from; 0 marks an import that keeps its package name.
	depth := make(map[string]int, len(paths))
	for _, importPath := range paths {
		if claims[declared[importPath]] > 1 {
			depth[importPath] = 2
		}
	}
	name := func(importPath string) string {
		if depth[importPath] == 0 {
			return declared[importPath]
		}
		return importName(importPath, depth[importPath])
	}
	for {
		// A reserved name is held by a framework import, entered as "" so
		// it counts toward a clash but never grows.
		byName := make(map[string][]string, len(paths)+len(reserved))
		for _, reservedName := range reserved {
			byName[reservedName] = append(byName[reservedName], "")
		}
		for _, importPath := range paths {
			n := name(importPath)
			byName[n] = append(byName[n], importPath)
		}
		grown := false
		for _, clashing := range byName {
			if len(clashing) < 2 {
				continue
			}
			for _, importPath := range clashing {
				if depth[importPath] > 0 && depth[importPath] <= strings.Count(importPath, "/") {
					depth[importPath]++
					grown = true
				}
			}
		}
		if !grown {
			break
		}
	}

	aliases := make(map[string]string, len(paths))
	taken := make(map[string]bool, len(paths)+len(reserved))
	for _, reservedName := range reserved {
		taken[reservedName] = true
	}
	for _, importPath := range paths {
		if depth[importPath] == 0 {
			aliases[importPath] = importSpecName(importPath, declared[importPath])
			taken[declared[importPath]] = true
		}
	}
	for _, importPath := range paths {
		if depth[importPath] == 0 {
			continue
		}
		alias := name(importPath)
		for n := 2; taken[alias]; n++ {
			alias = name(importPath) + strconv.Itoa(n)
		}
		aliases[importPath] = alias
		taken[alias] = true
	}
	return aliases
}

// importSpecName returns the name an import of importPath is written with when
// its file refers to the package as name: none when name is the last segment of
// the path, name otherwise, and name as well when that segment is a major
// version such as v2, which gopls skips for the segment before it when it
// assumes a name, and so names such an import on save.
// "helloworld/model/sample" referred to as sample is written without one;
// "helloworld/model/record_item" referred to as recorditem,
// "example.com/sample/v2" referred to as sample and "helloworld/model/api/v2"
// referred to as v2 are written with it.
func importSpecName(importPath, name string) string {
	base := path.Base(importPath)
	if name != base {
		return name
	}
	// gopls assumes the name with ImportPathToAssumedName, in a package
	// internal to x/tools (golang.org/x/tools/internal/imports), which takes
	// a v followed by what strconv.Atoi reads as a number for a major version.
	if rest, ok := strings.CutPrefix(base, "v"); ok {
		if _, err := strconv.Atoi(rest); err == nil {
			return name
		}
	}
	return ""
}

// importName joins the last depth segments of importPath with underscores, or
// all of them when it has fewer, into a Go identifier: every character an
// identifier cannot hold becomes an underscore, and a name that is still not
// one, because it starts with a digit or is a keyword, gets an underscore in
// front.
func importName(importPath string, depth int) string {
	segments := strings.Split(importPath, "/")
	name := strings.Map(func(r rune) rune {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, strings.Join(segments[max(len(segments)-depth, 0):], "_"))
	if !token.IsIdentifier(name) {
		name = "_" + name
	}
	return name
}
