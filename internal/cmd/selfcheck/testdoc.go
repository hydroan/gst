package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// docOpening matches the first word of a doc comment, a leading article
// passed over, the way godoclint's start-with-name rule reads it.
var docOpening = regexp.MustCompile(`^(?:(?:A|a|AN|An|an|THE|The|the) )?(\w+)`)

// checkTestDoc reports the doc comments of the test files whose first word is
// not the name of the declaration they document. golangci-lint's godoclint
// holds every other file to that, through its start-with-name rule, and skips
// test files with no switch to include them, so this check carries the rule
// over to them: a doc comment opening with another name is one an insertion
// between a neighbor's doc comment and its declaration took along. The rule
// reads as godoclint does: a leading article is passed over, and a blank
// identifier, a declaration naming several identifiers, a comment on a
// parenthesized group, a doc carrying a Deprecated paragraph, an Example
// function, whose doc describes what it demonstrates, and init, which has no
// name worth opening with, are left alone.
func checkTestDoc(root string, pkgs []*packages.Package) ([]violation, error) {
	type found struct {
		file, message string
		line          int
	}
	var all []found
	seen := make(map[string]bool)
	for _, p := range pkgs {
		for i, file := range p.Syntax {
			path := relative(root, p.CompiledGoFiles[i])
			if filepath.IsAbs(path) || !strings.HasSuffix(path, "_test.go") || ast.IsGenerated(file) || seen[path] {
				continue
			}
			seen[path] = true
			for _, decl := range file.Decls {
				for _, d := range documentedDecls(decl) {
					if d.name == "_" || d.name == "init" || strings.HasPrefix(d.name, "Example") {
						continue
					}
					text := d.doc.Text()
					if text == "" || hasDeprecatedParagraph(text) {
						continue
					}
					if m := docOpening.FindStringSubmatch(text); m != nil && m[1] == d.name {
						continue
					}
					pos := p.Fset.Position(d.doc.Pos())
					first, _, _ := strings.Cut(text, "\n")
					all = append(all, found{
						file:    path,
						line:    pos.Line,
						message: fmt.Sprintf("Test file '%s:%d': the doc comment of %s opens with %q; a doc comment opens with the name of what it documents", path, pos.Line, d.name, first),
					})
				}
			}
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].file != all[j].file {
			return all[i].file < all[j].file
		}
		return all[i].line < all[j].line
	})
	violations := make([]violation, 0, len(all))
	for _, f := range all {
		violations = append(violations, violation{File: f.file, Message: f.message})
	}
	return violations, nil
}

// documentedDecl is a declaration with the doc comment that documents it
// alone: a function, or a specification of a type, variable or constant,
// whose doc is the one on the specification, or on the declaration when it
// has no parentheses and so declares that one specification.
type documentedDecl struct {
	name string
	doc  *ast.CommentGroup
}

// documentedDecls returns the documented declarations of decl, in order: the
// function, or each specification naming one identifier with a doc of its
// own, a specification of a declaration without parentheses taking the
// declaration's doc.
func documentedDecls(decl ast.Decl) []documentedDecl {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Doc == nil {
			return nil
		}
		return []documentedDecl{{name: d.Name.Name, doc: d.Doc}}
	case *ast.GenDecl:
		var out []documentedDecl
		for _, spec := range d.Specs {
			var name string
			var doc *ast.CommentGroup
			switch s := spec.(type) {
			case *ast.TypeSpec:
				name, doc = s.Name.Name, s.Doc
			case *ast.ValueSpec:
				if len(s.Names) != 1 {
					continue
				}
				name, doc = s.Names[0].Name, s.Doc
			default:
				continue
			}
			if doc == nil && d.Lparen == token.NoPos {
				doc = d.Doc
			}
			if doc != nil {
				out = append(out, documentedDecl{name: name, doc: doc})
			}
		}
		return out
	default:
		return nil
	}
}

// hasDeprecatedParagraph reports whether a paragraph of the doc text opens
// with the Deprecated marker, which godoc renders apart from the rest, so the
// doc is read for its deprecation notice and nothing else is asked of it.
func hasDeprecatedParagraph(text string) bool {
	for paragraph := range strings.SplitSeq(text, "\n\n") {
		if strings.HasPrefix(paragraph, "Deprecated:") {
			return true
		}
	}
	return false
}
