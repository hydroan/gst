package ggcheck

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/hydroan/gst/internal/types"
)

// GRPCServiceContext keeps the services of the models served over gRPC to
// what both transports provide.
var GRPCServiceContext = Check{
	Name: "gRPC service context",
	Rule: "the services of models declaring GRPC() must not call the ServiceContext methods only HTTP serves: " + strings.Join(types.HTTPOnlyMethods, ", ") + "; the services of their Import, Export and SSE actions, served over HTTP alone, may",
	run:  checkGRPCServiceContext,
}

// checkGRPCServiceContext reports, for every model declaring GRPC(), each
// call of a method types.HTTPOnlyMethods names on a parameter declared
// *gst.ServiceContext that a gRPC call of the model reaches: over gRPC the
// call has no request to read or response to write, so a service serving
// both transports must do without it. A gRPC call enters the service type
// the service file of an action gRPC serves declares (see
// modelinfo.ServiceTarget): every method of that type, the hooks in
// whatever file declares them included, and the functions of that file;
// from there it reaches every function of the project they call, in the
// package or another, each call resolved the way the Go compiler reads it
// (see errDiscFuncScope.calleeOf), so a method of another type sharing a
// name is not taken for the one called. Left alone are the services of the
// actions gRPC does not serve (see dsl.HTTPOnlyAction), the helpers only
// those reach, and the files of models served over HTTP alone. A violation
// names the model whose call reaches it, the first of several; a file of
// such a service package the parser refuses is reported once, in its
// place.
func checkGRPCServiceContext(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	scanned, err := scanModels(ignore)
	if err != nil {
		return nil
	}
	analysis, err := collectProject(scanned.Module, ignore)
	if err != nil {
		return []string{fmt.Sprintf("walking project directory: %v", err)}
	}
	// Every function and method of the project by key, and every file by
	// path, for the calls to be followed.
	funcs := make(map[errDiscFuncKey]projectFunc)
	files := make(map[string]*errDiscFileCollector, len(analysis.files))
	for _, c := range analysis.files {
		files[c.path] = c
		for _, decl := range c.file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				funcs[errDiscFuncKey{pkgDir: c.pkgDir, recv: errDiscReceiverTypeName(receiverType(fn)), name: fn.Name.Name}] = projectFunc{file: c, decl: fn}
			}
		}
	}

	var found []violationAt
	reported := make(map[string]bool)
	served := make(map[string]bool)
	for _, m := range scanned.Models {
		if m.Design == nil || !m.Design.GRPC || ignore.Ignores(m.ModelFilePath, false) {
			continue
		}
		var entries []errDiscFuncKey
		m.Design.Range(func(_ string, act *dsl.Action) {
			if !act.Service || dsl.HTTPOnlyAction(act.Phase.Name()) {
				return
			}
			target := modelinfo.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			served[filepath.ToSlash(target.Dir)] = true
			// A service file yet to be generated, or one the parser refused,
			// holds no function to enter.
			c, ok := files[filepath.ToSlash(target.FilePath)]
			if !ok {
				return
			}
			for key, f := range funcs {
				if f.file.pkgDir == c.pkgDir && ((key.recv == "" && f.file == c) || slices.Contains(c.serviceTypes, key.recv)) {
					entries = append(entries, key)
				}
			}
		})
		visited := make(map[errDiscFuncKey]bool)
		for len(entries) > 0 {
			key := entries[0]
			entries = entries[1:]
			if visited[key] {
				continue
			}
			visited[key] = true
			f, ok := funcs[key]
			if !ok {
				continue
			}
			scope := f.file.newScope(f.decl)
			found = append(found, httpOnlyCalls(scope, m.ModelName, reported)...)
			ast.Inspect(f.decl.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if callee, ok := scope.calleeOf(call); ok && !visited[callee] {
						entries = append(entries, callee)
					}
				}
				return true
			})
		}
	}
	for file, parseErr := range analysis.parseErrors {
		if served[path.Dir(file)] {
			found = append(found, violationAt{pos: token.Position{Filename: file}, message: parseErr.Error()})
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].pos.Filename != found[j].pos.Filename {
			return found[i].pos.Filename < found[j].pos.Filename
		}
		return found[i].pos.Line < found[j].pos.Line
	})
	violations := make([]string, 0, len(found))
	for _, v := range found {
		violations = append(violations, v.message)
	}
	return violations
}

// projectFunc is a function or method of the project, with the file
// declaring it.
type projectFunc struct {
	file *errDiscFileCollector
	decl *ast.FuncDecl
}

// violationAt is a violation with the position it is reported at, for the
// report to be ordered by file and line.
type violationAt struct {
	pos     token.Position
	message string
}

// httpOnlyCalls lists the calls of the HTTP-only methods on the
// *gst.ServiceContext parameters of the function of scope, each as a
// violation naming the model whose call reaches the function, in source
// order; a call reported already, by an earlier model reaching the
// function, is left out, reported tracking them by position.
func httpOnlyCalls(scope *errDiscFuncScope, model string, reported map[string]bool) []violationAt {
	if len(scope.ctxParams) == 0 {
		return nil
	}
	var found []violationAt
	ast.Inspect(scope.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !scope.ctxParams[declObj(ident)] || !slices.Contains(types.HTTPOnlyMethods, sel.Sel.Name) {
			return true
		}
		pos := scope.file.analysis.fset.Position(call.Pos())
		at := fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Offset)
		if reported[at] {
			return true
		}
		reported[at] = true
		found = append(found, violationAt{pos: pos, message: fmt.Sprintf("%s:%d: calls %s.%s, which only HTTP serves; the model %s is served over gRPC as well, so keep its services to what both transports provide",
			filepath.ToSlash(pos.Filename), pos.Line, ident.Name, sel.Sel.Name, model)})
		return true
	})
	return found
}
