package pb

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/goast"
)

// This file holds what the generated Go files are assembled from: the
// builders of their syntax tree nodes, the imports of a file, and the file
// itself, which lays its declarations out by the positions go/printer
// breaks lines at.

// The packages the generated Go files import under fixed names, beside the
// project's own packages, which are named by gggen.ResolveImportConflicts
// around these.
const (
	importPathContext     = "context"
	importPathJSON        = "encoding/json"
	importPathHTTP        = "net/http"
	importPathTime        = "time"
	importPathDurationPB  = "google.golang.org/protobuf/types/known/durationpb"
	importPathStructPB    = "google.golang.org/protobuf/types/known/structpb"
	importPathTimestampPB = "google.golang.org/protobuf/types/known/timestamppb"
	importPathDatatypes   = "gorm.io/datatypes"
	importPathGorm        = "gorm.io/gorm"

	// gstModelName is the name the generated files import the framework's
	// model package under, a project's own model package keeping its own.
	gstModelName = "gstmodel"
)

// fixedImportNames maps the import paths of the packages imported under a
// fixed name to that name, "" for the package's own.
var fixedImportNames = map[string]string{
	importPathContext:        "",
	importPathJSON:           "",
	importPathHTTP:           "",
	importPathTime:           "",
	importPathDurationPB:     "",
	importPathStructPB:       "",
	importPathTimestampPB:    "",
	importPathDatatypes:      "",
	importPathGorm:           "",
	ggconst.ImportPathGRPC:   "",
	ggconst.ImportPathModel:  gstModelName,
	ggconst.ImportPathConsts: "",
}

// goImports collects the imports of one generated Go file while it is
// built: a fixed-name package is referred to by its name at once, and any
// other, a model package, another generated one or a package a model field
// takes its type from, through an identifier named by specs once every
// import is known, so that two packages of one name, model/record and
// pb/record, get aliases apart.
type goImports struct {
	fixed    map[string]bool         // the fixed-name packages used
	resolved map[string]string       // import path -> package name, of the others
	refs     map[string][]*ast.Ident // the identifiers referring to each resolved package
	blank    map[string]bool         // the packages imported for their initialization alone
}

func newGoImports() *goImports {
	return &goImports{fixed: make(map[string]bool), resolved: make(map[string]string), refs: make(map[string][]*ast.Ident), blank: make(map[string]bool)}
}

// blankImport records an import of the package at importPath for its
// initialization alone, which specs names _ unless the file refers to the
// package as well.
func (im *goImports) blankImport(importPath string) {
	im.blank[importPath] = true
}

// fixedRef returns the identifier the file refers to the fixed-name package
// at importPath by, recording the import.
func (im *goImports) fixedRef(importPath string) *ast.Ident {
	name, ok := fixedImportNames[importPath]
	if !ok {
		panic("pb: " + importPath + " is not a fixed-name import")
	}
	im.fixed[importPath] = true
	if name == "" {
		name = importPath[strings.LastIndex(importPath, "/")+1:]
	}
	return ast.NewIdent(name)
}

// ref returns an identifier referring to the package at importPath,
// declaring pkgName, one named by specs.
func (im *goImports) ref(importPath, pkgName string) *ast.Ident {
	im.resolved[importPath] = pkgName
	id := ast.NewIdent(pkgName)
	im.refs[importPath] = append(im.refs[importPath], id)
	return id
}

// specs names every resolved reference and returns the import specs of the
// file in three groups, the standard library, the resolved packages with the
// ones imported for their initialization alone, and the other fixed-name
// ones, each sorted by path, laid out on the lines of lines with a blank one
// between groups. A resolved package whose name a fixed-name import or
// another resolved import takes is aliased (see
// gggen.ResolveImportConflicts).
func (im *goImports) specs(lines *goast.LineSet) []ast.Spec {
	reserved := make([]string, 0, len(im.fixed))
	for importPath := range im.fixed {
		reserved = append(reserved, im.fixedRef(importPath).Name)
	}
	aliases := gggen.ResolveImportConflicts(im.resolved, reserved...)
	for importPath, alias := range aliases {
		if alias == "" {
			continue
		}
		for _, id := range im.refs[importPath] {
			id.Name = alias
		}
	}

	var std, others []string
	for importPath := range im.fixed {
		if strings.Contains(importPath, ".") {
			others = append(others, importPath)
		} else {
			std = append(std, importPath)
		}
	}
	slices.Sort(std)
	slices.Sort(others)
	resolved := slices.Sorted(maps.Keys(im.resolved))
	for importPath := range im.blank {
		if _, referred := im.resolved[importPath]; !referred {
			resolved = append(resolved, importPath)
		}
	}
	slices.Sort(resolved)

	var specs []ast.Spec
	for i, group := range [][]string{std, resolved, others} {
		if len(group) == 0 {
			continue
		}
		if len(specs) > 0 {
			lines.Next()
		}
		for _, importPath := range group {
			spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath), ValuePos: lines.Next()}}
			name := fixedImportNames[importPath]
			if i == 1 {
				if _, referred := im.resolved[importPath]; referred {
					name = aliases[importPath]
				} else {
					name = "_"
				}
			}
			if name != "" {
				spec.Name = &ast.Ident{Name: name, NamePos: spec.Path.ValuePos}
			}
			specs = append(specs, spec)
		}
	}
	return specs
}

// goDecl is one declaration of a generated Go file under its doc comment.
// layout, run as the file is laid out, positions the nodes of the
// declaration that go/printer breaks lines at: the elements of a composite
// literal spread over lines, the arguments of a call spread over lines.
type goDecl struct {
	doc    []string
	decl   ast.Decl
	layout func(lines *goast.LineSet)
}

// goFile is one generated Go file being assembled.
type goFile struct {
	pkgName string
	imports *goImports
	decls   []*goDecl
}

func newGoFile(pkgName string) *goFile {
	return &goFile{pkgName: pkgName, imports: newGoImports()}
}

// add appends a declaration under doc, wrapped into comment lines, laid
// out by layout, which may be nil.
func (f *goFile) add(doc string, decl ast.Decl, layout func(lines *goast.LineSet)) {
	f.decls = append(f.decls, &goDecl{doc: wrapComment(doc), decl: decl, layout: layout})
}

// source prints the file under the generated-code header: the imports, then
// every declaration under its doc comment, a blank line between them. The
// declarations are laid out on the lines of a fabricated file (see
// goast.LineSet): the comment lines and the keyword of a declaration take
// consecutive lines, its inner nodes the lines layout gives them, and the
// lines its printed form spans are then skipped, so that go/printer, which
// places a comment before the first token it estimates to lie past it,
// never reaches the next comment inside a declaration.
func (f *goFile) source() (string, error) {
	file := &ast.File{Name: ast.NewIdent(f.pkgName)}
	fset := gggen.GeneratedHeader(file)
	// The header and the package clause hold the first lines of the file
	// the header started; the lines of the declarations are a file of
	// their own, kept clear of those.
	lines := goast.NewLineSet(fset)
	skipLines(lines, 4)

	specs := f.imports.specs(lines)
	if len(specs) > 0 {
		lines.Next()
		file.Decls = append(file.Decls, &ast.GenDecl{TokPos: lines.Next(), Tok: token.IMPORT, Specs: specs})
	}
	for _, d := range f.decls {
		skipLines(lines, 2)
		if len(d.doc) > 0 {
			group := &ast.CommentGroup{}
			for _, line := range d.doc {
				group.List = append(group.List, &ast.Comment{Slash: lines.Next(), Text: "// " + line})
			}
			file.Comments = append(file.Comments, group)
		}
		setDeclPos(d.decl, lines.Next())
		if d.layout != nil {
			d.layout(lines)
		}
		var printed bytes.Buffer
		if err := format.Node(&printed, fset, d.decl); err != nil {
			return "", err
		}
		skipLines(lines, printed.Len()/lineSetWidth+2)
		file.Decls = append(file.Decls, d.decl)
	}
	return gggen.FormatNodeExtraWithFileSet(file, fset)
}

// lineSetWidth is the width of a line of a goast.LineSet in bytes, the
// unit source skips the printed form of a declaration by.
const lineSetWidth = 100

// skipLines takes n lines of lines without using them.
func skipLines(lines *goast.LineSet, n int) {
	for range n {
		lines.Next()
	}
}

// setDeclPos positions the keyword of decl at pos.
func setDeclPos(decl ast.Decl, pos token.Pos) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		d.Type.Func = pos
	case *ast.GenDecl:
		d.TokPos = pos
	}
}

// commentWidth is the width the doc comments of the generated files wrap at,
// the // prefix included.
const commentWidth = 77

// wrapComment wraps text into lines of at most commentWidth bytes, the //
// prefix counted; a word longer than a line takes one of its own.
func wrapComment(text string) []string {
	var lines []string
	var line string
	for word := range strings.FieldsSeq(text) {
		if line == "" {
			line = word
			continue
		}
		if len("// ")+len(line)+1+len(word) > commentWidth {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// The node builders below spell the generated code.

func ident(name string) *ast.Ident { return ast.NewIdent(name) }

func sel(x ast.Expr, name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: x, Sel: ast.NewIdent(name)}
}

func call(fn ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fn, Args: args}
}

func strLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

func star(x ast.Expr) *ast.StarExpr { return &ast.StarExpr{X: x} }

func addr(x ast.Expr) *ast.UnaryExpr { return &ast.UnaryExpr{Op: token.AND, X: x} }

func exprStmt(x ast.Expr) *ast.ExprStmt { return &ast.ExprStmt{X: x} }

// assign builds lhs = rhs.
func assign(lhs, rhs ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}
}

// define builds name := rhs, or name1, name2 := rhs.
func define(names []string, rhs ast.Expr) *ast.AssignStmt {
	lhs := make([]ast.Expr, len(names))
	for i, name := range names {
		lhs[i] = ident(name)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{rhs}}
}

func returns(results ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: results}
}

func block(stmts ...ast.Stmt) *ast.BlockStmt { return &ast.BlockStmt{List: stmts} }

// ifStmt builds if init; cond { body }, without init when it is nil.
func ifStmt(init ast.Stmt, cond ast.Expr, body ...ast.Stmt) *ast.IfStmt {
	return &ast.IfStmt{Init: init, Cond: cond, Body: block(body...)}
}

// ifNotNil builds if x != nil { body }.
func ifNotNil(x ast.Expr, body ...ast.Stmt) *ast.IfStmt {
	return ifStmt(nil, notNil(x), body...)
}

func notNil(x ast.Expr) *ast.BinaryExpr {
	return &ast.BinaryExpr{X: x, Op: token.NEQ, Y: ident("nil")}
}

// rangeStmt builds for key, value := range x { body }, with key alone when
// value is "" and neither when key is "" as well.
func rangeStmt(key, value string, x ast.Expr, body ...ast.Stmt) *ast.RangeStmt {
	stmt := &ast.RangeStmt{X: x, Body: block(body...)}
	if key != "" {
		stmt.Key = ident(key)
		stmt.Tok = token.DEFINE
	}
	if value != "" {
		stmt.Value = ident(value)
	}
	return stmt
}

// makeCall builds make(typ, n).
func makeCall(typ, n ast.Expr) *ast.CallExpr {
	return call(ident("make"), typ, n)
}

func lenCall(x ast.Expr) *ast.CallExpr { return call(ident("len"), x) }

func newCall(typ ast.Expr) *ast.CallExpr { return call(ident("new"), typ) }

func keyValue(key string, value ast.Expr) *ast.KeyValueExpr {
	return &ast.KeyValueExpr{Key: ident(key), Value: value}
}

func compositeLit(typ ast.Expr, elts ...ast.Expr) *ast.CompositeLit {
	return &ast.CompositeLit{Type: typ, Elts: elts}
}

func index(x, i ast.Expr) *ast.IndexExpr { return &ast.IndexExpr{X: x, Index: i} }

// layoutLiteral positions lit to print one element per line: its brace, then
// each element, then its closing brace, each on the next line of lines.
func layoutLiteral(lit *ast.CompositeLit, lines *goast.LineSet) {
	lit.Lbrace = lines.Next()
	for _, elt := range lit.Elts {
		setExprPos(elt, lines.Next())
	}
	lit.Rbrace = lines.Next()
}

// setExprPos positions the first token of x at pos.
func setExprPos(x ast.Expr, pos token.Pos) {
	switch e := x.(type) {
	case *ast.Ident:
		e.NamePos = pos
	case *ast.KeyValueExpr:
		setExprPos(e.Key, pos)
	case *ast.SelectorExpr:
		setExprPos(e.X, pos)
	case *ast.CallExpr:
		setExprPos(e.Fun, pos)
	case *ast.CompositeLit:
		if e.Type != nil {
			setExprPos(e.Type, pos)
		} else {
			e.Lbrace = pos
		}
	case *ast.FuncLit:
		e.Type.Func = pos
	case *ast.BasicLit:
		e.ValuePos = pos
	case *ast.StarExpr:
		e.Star = pos
	case *ast.UnaryExpr:
		e.OpPos = pos
	case *ast.IndexExpr:
		setExprPos(e.X, pos)
	default:
		panic(fmt.Sprintf("pb: cannot position a %T", x))
	}
}

// funcLit builds a function literal taking params, returning results and
// running stmts.
func funcLit(params, results []*ast.Field, stmts ...ast.Stmt) *ast.FuncLit {
	return &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{List: params}, Results: &ast.FieldList{List: results}}, Body: block(stmts...)}
}
