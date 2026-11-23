package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// translatedLanguagesVar names the variable of the translator package
// listing the languages the framework speaks, the one its sentences are
// written in first.
const translatedLanguagesVar = "languages"

// checkTranslations holds every fixed sentence a client is answered with to
// a translation into each language the framework speaks beside the one its
// sentences are written in (see translatedLanguages), registered by the
// package the sentence belongs to: a client reading that language is
// otherwise answered in English by the one refusal whose translation was
// forgotten.
//
// A sentence is a string constant handed to a function taking one (see
// sentenceUses). It belongs to the package declaring the constant handed
// over, followed through the constants that only rename another, and a
// string written at the call belongs to the package of the call.
//
// A package registers its translations in its translations.go, each under a
// constant that file declares: a sentence and its translations stand in one
// file, and a translation follows its sentence wherever it is reworded.
//
// Reported are a sentence its package registers no translation for in one
// of the languages; a registration written in another file; a registration
// the check cannot read, one not handed a constant language and a map
// literal; a translation registered under anything but a constant its file
// declares; and a translation under a constant nothing but the
// registrations uses, the sentence no client is answered with.
// Test files, which only the test variants of a package hold, are left
// alone.
func checkTranslations(root string, pkgs []*packages.Package) ([]violation, error) {
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, nil
	}
	module := pkgs[0].Module.Path
	plain := plainPackages(pkgs)
	languages, err := translatedLanguages(module, plain)
	if err != nil {
		return nil, err
	}

	// declared maps a constant to the expression declaring its value, by
	// which owner follows a constant that renames another.
	declared := make(map[types.Object]ast.Expr)
	for _, p := range plain {
		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range value.Names {
						if i < len(value.Values) {
							declared[p.TypesInfo.Defs[name]] = value.Values[i]
						}
					}
				}
			}
		}
	}
	infoOf := make(map[*types.Package]*types.Info, len(plain))
	for _, p := range plain {
		infoOf[p.Types] = p.TypesInfo
	}
	// owner returns the constant expr names, followed to the one that holds
	// the string rather than renaming another, or nil when expr names none.
	var owner func(info *types.Info, expr ast.Expr) *types.Const
	owner = func(info *types.Info, expr ast.Expr) *types.Const {
		named, ok := usedObject(info, expr).(*types.Const)
		if !ok {
			return nil
		}
		if value, ok := declared[named]; ok {
			if renamed := owner(infoOf[named.Pkg()], value); renamed != nil {
				return renamed
			}
		}
		return named
	}

	type place struct {
		file string
		line int
	}
	sentences := make(map[string]map[string]place) // package -> sentence -> where it is declared or written
	for _, use := range sentenceUses(module, plain) {
		pkgPath, pos := use.pkg.PkgPath, use.pkg.Fset.Position(use.arg.Pos())
		if named := owner(use.pkg.TypesInfo, use.arg); named != nil && named.Pkg() != nil && infoOf[named.Pkg()] != nil {
			pkgPath, pos = named.Pkg().Path(), use.pkg.Fset.Position(named.Pos())
		}
		if sentences[pkgPath] == nil {
			sentences[pkgPath] = make(map[string]place)
		}
		if _, seen := sentences[pkgPath][use.text]; !seen {
			sentences[pkgPath][use.text] = place{file: relative(root, pos.Filename), line: pos.Line}
		}
	}

	type translation struct {
		language string
		text     string
		at       place
	}
	translated := make(map[string]map[string]map[string]bool) // package -> language -> sentence
	keyed := make(map[*types.Const][]translation)             // the translations registered under a constant
	keys := make(map[*ast.Ident]bool)                         // the identifiers naming those constants as keys
	var found []violation
	for _, registration := range translationRegistrations(module, plain) {
		p, call := registration.pkg, registration.call
		at := p.Fset.Position(call.Pos())
		file := relative(root, at.Filename)
		if filepath.Base(at.Filename) != sentenceFile {
			found = append(found, violation{
				File: file,
				Message: fmt.Sprintf("Registration at %s:%d is written outside %s: a package registers its translations in the file of that name",
					file, at.Line, sentenceFile),
			})
		}
		unreadable := violation{
			File: file,
			Message: fmt.Sprintf("Registration at %s:%d is not handed a constant language and a map literal: write the translations at the call, where the check reads them",
				file, at.Line),
		}
		if len(call.Args) != 2 {
			found = append(found, unreadable)
			continue
		}
		language, ok := constantString(p.TypesInfo, call.Args[0])
		literal, isLiteral := ast.Unparen(call.Args[1]).(*ast.CompositeLit)
		if !ok || !isLiteral {
			found = append(found, unreadable)
			continue
		}
		if translated[p.PkgPath] == nil {
			translated[p.PkgPath] = make(map[string]map[string]bool)
		}
		if translated[p.PkgPath][language] == nil {
			translated[p.PkgPath][language] = make(map[string]bool)
		}
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				found = append(found, unreadable)
				break
			}
			text, ok := constantString(p.TypesInfo, pair.Key)
			if !ok {
				found = append(found, unreadable)
				break
			}
			translated[p.PkgPath][language][text] = true
			pos := p.Fset.Position(pair.Key.Pos())
			here := place{file: relative(root, pos.Filename), line: pos.Line}
			ident, _ := ast.Unparen(pair.Key).(*ast.Ident)
			var named *types.Const
			if ident != nil {
				named, _ = p.TypesInfo.Uses[ident].(*types.Const)
			}
			if named == nil || p.Fset.Position(named.Pos()).Filename != pos.Filename {
				found = append(found, violation{
					File: here.file,
					Message: fmt.Sprintf("Translation into %s of %q at %s:%d is not registered under a constant its file declares: declare the sentence in that file and register the translation under the constant",
						language, text, here.file, here.line),
				})
				continue
			}
			keys[ident] = true
			keyed[named] = append(keyed[named], translation{language: language, text: text, at: here})
		}
	}

	for pkgPath, declaredHere := range sentences {
		for text, at := range declaredHere {
			for _, language := range languages {
				if translated[pkgPath][language][text] {
					continue
				}
				found = append(found, violation{
					File: at.file,
					Message: fmt.Sprintf("Sentence %q at %s:%d is answered to clients and has no %s translation: register one in the %s of %s, the package it belongs to",
						text, at.file, at.line, language, sentenceFile, strings.TrimPrefix(pkgPath, module+"/")),
				})
			}
		}
	}

	// answered holds the constants a translation is registered under that a
	// file uses for more than the registrations: the registration keeps a
	// constant in use for the compiler and the linters, so the sentence
	// nothing answers with is told from the others here.
	answered := make(map[*types.Const]bool)
	for _, p := range plain {
		for ident, obj := range p.TypesInfo.Uses {
			if named, ok := obj.(*types.Const); ok && keyed[named] != nil && !keys[ident] {
				answered[named] = true
			}
		}
	}
	for named, registered := range keyed {
		if answered[named] {
			continue
		}
		for _, entry := range registered {
			found = append(found, violation{
				File: entry.at.file,
				Message: fmt.Sprintf("Translation into %s of %q at %s:%d is registered under %s, which nothing else uses: drop the constant and its translations, no client is answered with the sentence",
					entry.language, entry.text, entry.at.file, entry.at.line, named.Name()),
			})
		}
	}

	slices.SortFunc(found, func(a, b violation) int {
		return cmp.Or(strings.Compare(a.File, b.File), strings.Compare(a.Message, b.Message))
	})
	return found, nil
}

// translatedLanguages returns the languages a sentence of the module is
// translated into: the elements, after the first, of the list its translator
// package declares under translatedLanguagesVar, the first being the
// language the sentences are written in. The list is read from the source
// of the module checked rather than from the package the check is built
// with, so a module is held to the languages it speaks itself. A module
// with no translator package translates into none.
func translatedLanguages(module string, plain []*packages.Package) ([]string, error) {
	for _, p := range plain {
		if p.PkgPath != module+"/"+sentenceTranslatorPkg {
			continue
		}
		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.VAR {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range value.Names {
						if name.Name != translatedLanguagesVar || i >= len(value.Values) {
							continue
						}
						literal, ok := ast.Unparen(value.Values[i]).(*ast.CompositeLit)
						if !ok || len(literal.Elts) == 0 {
							return nil, errors.Newf("%s declares %s as no list of languages", sentenceTranslatorPkg, translatedLanguagesVar)
						}
						languages := make([]string, 0, len(literal.Elts))
						for _, element := range literal.Elts {
							language, ok := constantString(p.TypesInfo, element)
							if !ok {
								return nil, errors.Newf("%s lists a language in %s that is no string constant", sentenceTranslatorPkg, translatedLanguagesVar)
							}
							languages = append(languages, language)
						}
						return languages[1:], nil
					}
				}
			}
		}
		return nil, errors.Newf("%s declares no %s to read the languages of the framework from", sentenceTranslatorPkg, translatedLanguagesVar)
	}
	return nil, nil
}
