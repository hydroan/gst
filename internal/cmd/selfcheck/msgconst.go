package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// msgConstSuffix ends the name of a constant holding a sentence a client is
// answered with.
const msgConstSuffix = "Msg"

// checkMsgConst holds every fixed sentence a client is answered with to a
// constant of the framework whose name ends in Msg. The constant is what the
// translations of the sentence are registered under (see checkTranslations),
// so a sentence written where it is answered has none to follow it, and it
// is written again wherever it is answered once more; the one suffix tells
// a reader the sentences of a package from its other strings.
//
// Reported are a string constant handed to a function taking a sentence (see
// sentenceUses) that no constant of the framework holds, a string written at
// the call, one put together from constants there, or a constant of another
// module; and a constant handed to one, or a translation is registered
// under, whose name does not end in Msg, once, where it is declared. Test
// files, which only the test variants of a package hold, are left alone.
func checkMsgConst(root string, pkgs []*packages.Package) ([]violation, error) {
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, nil
	}
	module := pkgs[0].Module.Path
	plain := plainPackages(pkgs)
	ofModule := make(map[*types.Package]bool, len(plain))
	for _, p := range plain {
		ofModule[p.Types] = true
	}

	var found []violation
	misnamed := make(map[*types.Const]bool)
	// named returns the constant of the framework expr names, reporting it
	// the first time its name does not end in Msg, or nil when expr names
	// none.
	named := func(p *packages.Package, expr ast.Expr) *types.Const {
		constant, ok := usedObject(p.TypesInfo, expr).(*types.Const)
		if !ok || !ofModule[constant.Pkg()] {
			return nil
		}
		if !strings.HasSuffix(constant.Name(), msgConstSuffix) && !misnamed[constant] {
			misnamed[constant] = true
			pos := p.Fset.Position(constant.Pos())
			file := relative(root, pos.Filename)
			found = append(found, violation{
				File: file,
				Message: fmt.Sprintf("Constant %s at %s:%d holds a sentence a client is answered with: end its name in %s",
					constant.Name(), file, pos.Line, msgConstSuffix),
			})
		}
		return constant
	}

	for _, use := range sentenceUses(module, plain) {
		if named(use.pkg, use.arg) != nil {
			continue
		}
		pos := use.pkg.Fset.Position(use.arg.Pos())
		file := relative(root, pos.Filename)
		found = append(found, violation{
			File: file,
			Message: fmt.Sprintf("Sentence %q at %s:%d is answered with and held by no constant of the framework: declare it as a constant in the %s of its package and answer with the constant",
				use.text, file, pos.Line, sentenceFile),
		})
	}
	for _, registration := range translationRegistrations(module, plain) {
		if len(registration.call.Args) != 2 {
			continue
		}
		literal, ok := ast.Unparen(registration.call.Args[1]).(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, element := range literal.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				named(registration.pkg, pair.Key)
			}
		}
	}

	slices.SortFunc(found, func(a, b violation) int {
		return cmp.Or(strings.Compare(a.File, b.File), strings.Compare(a.Message, b.Message))
	})
	return found, nil
}
