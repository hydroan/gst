package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// constLiteralHome is the package, by its path under the module, whose string
// constants spell the names the tooling shares: a package importing it spells
// them through it.
const constLiteralHome = "internal/ggconst"

// literalDirective opens the comment closing the line of a string that is
// spelt like a constant of constLiteralHome and means something else, with
// what it means after it: //gst:literal <what the string means>.
const literalDirective = "//gst:literal"

// constLiteralSinks are the packages outside the module that take a name or a
// path of the tooling's as a string. A string handed to one of them is the
// module's own spelling; one handed to any other outside package, a command
// line library or os/exec, is a word of that package's vocabulary.
var constLiteralSinks = map[string]bool{
	"bytes":         true,
	"go/ast":        true,
	"os":            true,
	"path":          true,
	"path/filepath": true,
	"strings":       true,
}

// checkConstLiteral reports the string literals that spell, in full, an
// exported string constant of constLiteralHome, in the non-test files of the
// packages under root that import it: a name the tooling shares is spelt
// once, there, and a literal repeating it parts from the constant the day one
// of the two changes. A string only containing the spelling is its own. Left
// alone are the imports, the constant declarations, which give the string a
// name of its own, and the strings handed as they stand, an argument or a
// field of a composite literal, to code declared outside the module other
// than constLiteralSinks. A string spelt like a constant that means
// something else closes its line with a literalDirective, held to its word:
// one without a reason is reported, and so is one on a line where the check
// reports no string.
//
// golangci-lint's goconst matches a string against the constants its own
// package declares and no others, so it cannot hold a package to the
// constants of another.
func checkConstLiteral(root string, pkgs []*packages.Package) ([]violation, error) {
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, nil
	}
	module := pkgs[0].Module.Path
	home := module + "/" + constLiteralHome

	var homeName string
	spelling := make(map[string][]string) // the value of a constant -> the constants of that value
	for _, p := range pkgs {
		if p.ForTest != "" || p.PkgPath != home {
			continue
		}
		homeName = p.Name
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			c, ok := scope.Lookup(name).(*types.Const)
			if !ok || !c.Exported() || c.Val().Kind() != constant.String {
				continue
			}
			value := constant.StringVal(c.Val())
			spelling[value] = append(spelling[value], homeName+"."+name)
		}
	}
	if len(spelling) == 0 {
		return nil, nil
	}

	type finding struct {
		file    string
		line    int
		message string
	}
	// directive is a literalDirective, with whether the check found a string
	// to report on the line it closes.
	type directive struct {
		reason string
		used   bool
	}
	var found []finding
	for _, p := range pkgs {
		if p.ForTest != "" || p.PkgPath == home || p.Imports[home] == nil {
			continue
		}
		for _, file := range p.Syntax {
			rel := relative(root, p.Fset.Position(file.Pos()).Filename)
			if isTestFile(rel) {
				continue
			}

			directives := make(map[int]*directive) // by the line each closes
			for _, group := range file.Comments {
				for _, c := range group.List {
					rest, ok := strings.CutPrefix(c.Text, literalDirective)
					if !ok || (rest != "" && !strings.HasPrefix(rest, " ")) {
						continue
					}
					directives[p.Fset.Position(c.Slash).Line] = &directive{reason: strings.TrimSpace(rest)}
				}
			}

			var parents []ast.Node
			ast.Inspect(file, func(n ast.Node) bool {
				if n == nil {
					parents = parents[:len(parents)-1]
					return true
				}
				switch n := n.(type) {
				case *ast.ImportSpec:
					return false
				case *ast.GenDecl:
					if n.Tok == token.CONST {
						return false
					}
				case *ast.BasicLit:
					if n.Kind != token.STRING {
						return false
					}
					value, err := strconv.Unquote(n.Value)
					if err != nil || spelling[value] == nil || handedOutside(module, p.TypesInfo, parents) {
						return false
					}
					line := p.Fset.Position(n.Pos()).Line
					if d := directives[line]; d != nil {
						d.used = true
						return false
					}
					use := "the constant"
					if len(spelling[value]) > 1 {
						use = "the one it means"
					}
					found = append(found, finding{file: rel, line: line, message: fmt.Sprintf(
						"String %q at %s:%d spells %s: use %s, or close the line with %s <what the string means> when it means something else",
						value, rel, line, strings.Join(spelling[value], " or "), use, literalDirective)})
					return false
				}
				parents = append(parents, n)
				return true
			})

			for line, d := range directives {
				switch {
				case d.reason == "":
					found = append(found, finding{file: rel, line: line, message: fmt.Sprintf(
						"%s at %s:%d gives no reason: say what the string means after the directive", literalDirective, rel, line)})
				case !d.used:
					found = append(found, finding{file: rel, line: line, message: fmt.Sprintf(
						"%s at %s:%d speaks for no string the check reports: drop the directive", literalDirective, rel, line)})
				}
			}
		}
	}

	slices.SortFunc(found, func(a, b finding) int {
		return cmp.Or(strings.Compare(a.file, b.file), cmp.Compare(a.line, b.line), strings.Compare(a.message, b.message))
	})
	violations := make([]violation, 0, len(found))
	for _, f := range found {
		violations = append(violations, violation{File: f.file, Message: f.message})
	}
	return violations, nil
}

// handedOutside reports whether the string whose ancestors are parents, the
// innermost last, is handed as it stands to code declared outside module
// other than constLiteralSinks: an argument of such a function or method, or
// a field of a composite literal of such a type. Parentheses and a
// conversion around the string are seen through, so the string of
// bytes.Contains(src, []byte("init")) is handed to bytes.
func handedOutside(module string, info *types.Info, parents []ast.Node) bool {
	outside := func(obj types.Object) bool {
		if obj == nil || obj.Pkg() == nil {
			return false
		}
		path := obj.Pkg().Path()
		return path != module && !strings.HasPrefix(path, module+"/") && !constLiteralSinks[path]
	}
	for _, node := range slices.Backward(parents) {
		switch parent := node.(type) {
		case *ast.ParenExpr, *ast.KeyValueExpr:
		case *ast.CallExpr:
			if tv, ok := info.Types[parent.Fun]; ok && tv.IsType() {
				continue
			}
			// The function called, without the type arguments of an
			// explicit instantiation.
			fun := ast.Unparen(parent.Fun)
			switch index := fun.(type) {
			case *ast.IndexExpr:
				fun = index.X
			case *ast.IndexListExpr:
				fun = index.X
			}
			switch fun := fun.(type) {
			case *ast.Ident:
				return outside(info.Uses[fun])
			case *ast.SelectorExpr:
				return outside(info.Uses[fun.Sel])
			}
			return false
		case *ast.CompositeLit:
			named, ok := derefNamed(info.TypeOf(parent))
			return ok && outside(named.Obj())
		default:
			return false
		}
	}
	return false
}
