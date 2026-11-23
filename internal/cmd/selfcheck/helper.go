package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// The helpers more than one check uses, or a check shares with run. A helper
// only one check uses lives in that check's file.

// load loads the packages matching patterns under dir with their tests,
// syntax, type and module information, reading the overlay in place of the
// files it names.
func load(dir string, overlay map[string][]byte, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedForTest | packages.NeedModule,
		Tests:   true,
		Dir:     dir,
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, errors.Wrap(err, "load packages")
	}
	return pkgs, nil
}

// isInternalTestVariant reports whether p is a package compiled together with
// its internal test files, as go test builds it: the variant of a package
// built for its own tests. It asks ForTest rather than reading the package ID,
// whose syntax go/packages leaves to the build system and tells clients not to
// interpret.
func isInternalTestVariant(p *packages.Package) bool {
	return p.ForTest != "" && p.ForTest == p.PkgPath
}

// relative returns path relative to root in slash form, or path itself when it
// lies outside root.
func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// derefNamed returns the named type t is, or points to.
func derefNamed(t types.Type) (*types.Named, bool) {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	return named, ok
}

// The checks on the sentences a client is answered with find them by these:
// the packages of the module, by their path under it, and the file a package
// keeps its sentences in.
const (
	// sentenceErrorsPkg declares the constructors of the error a request is
	// answered with, which take its sentence.
	sentenceErrorsPkg = "internal/types"
	// sentenceTranslatorPkg registers the translations and translates a
	// sentence at the moment a transport answers with it.
	sentenceTranslatorPkg = "internal/i18n"
	// sentenceForwarderPkg is the public package forwarding the registration
	// to the module sources, which import no internal package.
	sentenceForwarderPkg = "i18n"
	// sentenceRegister names the registration in both.
	sentenceRegister = "Register"
	// sentenceFile names the file a package declares its sentences in and
	// registers their translations from.
	sentenceFile = "translations.go"
)

// sinkSeed names a function taking a sentence a client is answered with, by
// its package, its name and the index of the parameter taking the sentence.
type sinkSeed struct {
	pkg   string
	name  string
	param int
}

// errorConstructors seeds the constructors of the error a request is
// answered with, the service error: a status and a sentence for the client,
// which the transports answer with when it is the outermost of its kind in
// the error they are handed.
var errorConstructors = []sinkSeed{
	{sentenceErrorsPkg, "NewError", 1},
	{sentenceErrorsPkg, "NewErrorWithCause", 1},
}

// sentenceSinkSeeds seeds the functions taking a sentence a client is
// answered with: the constructors of the service error and the translation
// itself. A function handing a parameter of its own to one of them, and a
// variable holding one, takes a sentence the same way (see sentenceSinks).
var sentenceSinkSeeds = append(slices.Clone(errorConstructors), sinkSeed{sentenceTranslatorPkg, "Translate", 0})

// plainPackages returns the packages of pkgs as a program builds them,
// leaving out the variants go test builds, the only ones holding test files.
func plainPackages(pkgs []*packages.Package) []*packages.Package {
	var plain []*packages.Package
	for _, p := range pkgs {
		if p.ForTest == "" && !strings.HasSuffix(p.PkgPath, ".test") {
			plain = append(plain, p)
		}
	}
	return plain
}

// sentenceUse is a string constant handed to a function taking a sentence a
// client is answered with.
type sentenceUse struct {
	// pkg is the package of the call.
	pkg *packages.Package
	// arg is the expression handed over.
	arg ast.Expr
	// text is the sentence.
	text string
}

// sentenceUses returns the string constants the packages of plain hand to a
// function taking a sentence a client is answered with (see sentenceSinks),
// in the order the packages and their files hold them: a constructor of the
// error a request is answered with, the translation itself, or a function or
// variable forwarding to them. A message put together as the request is
// served, which no constant holds, is none, and neither is the empty string.
func sentenceUses(module string, plain []*packages.Package) []sentenceUse {
	sinks := sentenceSinks(module, plain, sentenceSinkSeeds)
	var uses []sentenceUse
	for _, p := range plain {
		for _, file := range p.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				for _, param := range sinks[usedObject(p.TypesInfo, call.Fun)] {
					if param >= len(call.Args) {
						continue
					}
					if text, ok := constantString(p.TypesInfo, call.Args[param]); ok && text != "" {
						uses = append(uses, sentenceUse{pkg: p, arg: call.Args[param], text: text})
					}
				}
				return true
			})
		}
	}
	return uses
}

// sentenceSinks returns the functions and variables of pkgs taking a
// sentence a client is answered with, each with the indexes of the
// parameters taking one: the functions of seeds; a function handing a
// parameter of its own, as it stands, to one of them, the helper a package
// answers its refusals through; and a package variable holding one, the form
// a public package forwards a constructor in. They are found in rounds, a
// round adding what forwards to the ones before it, until a round adds none.
func sentenceSinks(module string, pkgs []*packages.Package, seeds []sinkSeed) map[types.Object][]int {
	sinks := make(map[types.Object][]int)
	for _, p := range pkgs {
		for _, seed := range seeds {
			if p.PkgPath != module+"/"+seed.pkg {
				continue
			}
			if fn := p.Types.Scope().Lookup(seed.name); fn != nil {
				sinks[fn] = []int{seed.param}
			}
		}
	}
	add := func(obj types.Object, param int) bool {
		if obj == nil || slices.Contains(sinks[obj], param) {
			return false
		}
		sinks[obj] = append(sinks[obj], param)
		return true
	}
	for grew := true; grew; {
		grew = false
		for _, p := range pkgs {
			for _, file := range p.Syntax {
				for _, decl := range file.Decls {
					switch d := decl.(type) {
					case *ast.FuncDecl:
						if d.Body == nil {
							continue
						}
						params := make(map[types.Object]int)
						index := 0
						for _, field := range d.Type.Params.List {
							for _, name := range field.Names {
								params[p.TypesInfo.Defs[name]] = index
								index++
							}
							if len(field.Names) == 0 {
								index++
							}
						}
						ast.Inspect(d.Body, func(n ast.Node) bool {
							call, ok := n.(*ast.CallExpr)
							if !ok {
								return true
							}
							for _, param := range sinks[usedObject(p.TypesInfo, call.Fun)] {
								if param >= len(call.Args) {
									continue
								}
								handed, ok := ast.Unparen(call.Args[param]).(*ast.Ident)
								if !ok {
									continue
								}
								if own, ok := params[p.TypesInfo.Uses[handed]]; ok && add(p.TypesInfo.Defs[d.Name], own) {
									grew = true
								}
							}
							return true
						})
					case *ast.GenDecl:
						if d.Tok != token.VAR {
							continue
						}
						for _, spec := range d.Specs {
							value, ok := spec.(*ast.ValueSpec)
							if !ok {
								continue
							}
							for i, name := range value.Names {
								if i >= len(value.Values) {
									continue
								}
								for _, param := range sinks[usedObject(p.TypesInfo, value.Values[i])] {
									if add(p.TypesInfo.Defs[name], param) {
										grew = true
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return sinks
}

// translationRegistration is a call registering the translations of a
// package.
type translationRegistration struct {
	// pkg is the package of the call.
	pkg *packages.Package
	// call is the call.
	call *ast.CallExpr
}

// translationRegistrations returns the calls among the packages of plain
// that register translations, with internal/i18n or with the public package
// forwarding to it, in the order the packages and their files hold them.
// The calls of those two packages are left out: they hand the registration
// on rather than register.
func translationRegistrations(module string, plain []*packages.Package) []translationRegistration {
	translator, forwarder := module+"/"+sentenceTranslatorPkg, module+"/"+sentenceForwarderPkg
	var registrations []translationRegistration
	for _, p := range plain {
		if p.PkgPath == translator || p.PkgPath == forwarder {
			continue
		}
		for _, file := range p.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				register, ok := usedObject(p.TypesInfo, call.Fun).(*types.Func)
				if ok && register.Pkg() != nil && register.Name() == sentenceRegister &&
					(register.Pkg().Path() == translator || register.Pkg().Path() == forwarder) {
					registrations = append(registrations, translationRegistration{pkg: p, call: call})
				}
				return true
			})
		}
	}
	return registrations
}

// usedObject returns the object expr names: the one an identifier or the
// selection of a package member uses, behind parentheses and the type
// arguments of an instantiation; nil for any other expression.
func usedObject(info *types.Info, expr ast.Expr) types.Object {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		return info.Uses[e.Sel]
	case *ast.IndexExpr:
		return usedObject(info, e.X)
	case *ast.IndexListExpr:
		return usedObject(info, e.X)
	}
	return nil
}

// constantString returns the string expr is a constant of, and false for an
// expression that is no string constant.
func constantString(info *types.Info, expr ast.Expr) (string, bool) {
	value := info.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}
