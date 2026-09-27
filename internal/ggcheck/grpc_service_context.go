package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/hydroan/gst/internal/types"
)

// GRPCServiceContext keeps the services of the models served over gRPC to
// what both transports provide.
var GRPCServiceContext = Check{
	Name: "gRPC service context",
	Rule: "the service packages of models declaring GRPC() must not call the ServiceContext methods only HTTP serves: " + strings.Join(types.HTTPOnlyMethods, ", "),
	run:  checkGRPCServiceContext,
}

// checkGRPCServiceContext reports, in every service package of a model
// declaring GRPC(), each call of a method types.HTTPOnlyMethods names on a
// parameter declared *gst.ServiceContext: over gRPC the call has no request
// to read or response to write, so a service serving both transports must do
// without it. The packages are the ones the actions of the model map to
// (see modelinfo.ServiceTarget), the helpers a service method reaches
// included (see httpOnlyCallsReached); left alone are the service files of
// the actions gRPC does not serve (see dsl.HTTPOnlyAction), an SSE service
// calling ctx.SSE as it must, the helpers only those reach, and the files of
// models served over HTTP alone that share the package. The analysis is
// syntactic like the other checks: it follows the parameter object, so a
// local variable of the same name is not mistaken for it, it does not
// follow the context into a variable assigned from it, and it reaches a
// helper by the name it is called by.
func checkGRPCServiceContext(ignore gghelper.ProjectIgnore) []string {
	var violations []string
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}
	cfg, err := ggconfig.Load(".")
	if err != nil {
		return append(violations, fmt.Sprintf("loading gst.yaml: %v", err))
	}
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return append(violations, fmt.Sprintf("reading the module path: %v", err))
	}
	allModels, err := modelinfo.FindModels(modulePath, ggconst.DirModel, ignore)
	if err != nil {
		return append(violations, fmt.Sprintf("scanning model designs: %v", err))
	}
	modelinfo.ResolveRoutes(allModels, cfg.Gen.Routes.Ignore)

	// Every action's service file, with whether gRPC serves the action, and
	// the service directories of the gRPC models, each with the model it
	// serves, in the order the models were found.
	served := make(map[string]bool)
	var dirs []string
	owners := make(map[string]string)
	for _, m := range allModels {
		if m.Design == nil || ignore.Ignores(m.ModelFilePath, false) {
			continue
		}
		m.Design.Range(func(_ string, act *dsl.Action) {
			if !act.Service {
				return
			}
			target := modelinfo.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			served[target.FilePath] = m.Design.GRPC && !dsl.HTTPOnlyAction(act.Phase.Name())
			if !m.Design.GRPC {
				return
			}
			if _, seen := owners[target.Dir]; !seen {
				dirs = append(dirs, target.Dir)
				owners[target.Dir] = m.ModelName
			}
		})
	}
	for _, dir := range dirs {
		violations = append(violations, httpOnlyCallsReached(dir, owners[dir], served, ignore)...)
	}
	return violations
}

// packageFunc is a top-level function or method of a service package, with
// the file it is declared in.
type packageFunc struct {
	path     string
	fset     *token.FileSet
	decl     *ast.FuncDecl
	gstNames goast.PackageNames
}

// httpOnlyCallsReached lists the calls of the HTTP-only methods in the
// functions of the service package at dir a gRPC call reaches, each as a
// violation naming the model that owns the package, in file and source
// order: the functions of the service files of the actions gRPC serves,
// served[path] being true, and every function of the package those call,
// directly or through others, by name. A helper only the service of an SSE
// or an Export action calls may use what only HTTP serves, as that service
// may itself (see checkGRPCServiceContext); one nothing calls is left
// alone too. A file that fails to parse is reported as one violation.
func httpOnlyCallsReached(dir, model string, served map[string]bool, ignore gghelper.ProjectIgnore) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var funcs []packageFunc
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		if entry.IsDir() || !strings.HasSuffix(name, ggconst.ExtensionGo) || strings.HasSuffix(name, ggconst.PatternTestFile) || ignore.Ignores(path, false) {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		gstNames := goast.ImportedNames(file, gstImportPath, "gst")
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				funcs = append(funcs, packageFunc{path: path, fset: fset, decl: fn, gstNames: gstNames})
			}
		}
	}

	// The functions a gRPC call reaches: those of the served action files,
	// then whatever they call, by the name of the function or method.
	byName := make(map[string][]int, len(funcs))
	for i, f := range funcs {
		byName[f.decl.Name.Name] = append(byName[f.decl.Name.Name], i)
	}
	reached := make([]bool, len(funcs))
	var queue []int
	for i, f := range funcs {
		if grpc, action := served[f.path]; action && grpc {
			reached[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		ast.Inspect(funcs[i].decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			for _, j := range byName[name] {
				if !reached[j] {
					reached[j] = true
					queue = append(queue, j)
				}
			}
			return true
		})
	}

	for i, f := range funcs {
		if reached[i] {
			violations = append(violations, httpOnlyCalls(f, model)...)
		}
	}
	return violations
}

// httpOnlyCalls lists the calls of the HTTP-only methods in the function f,
// each as a violation naming the model that owns the package, in source
// order.
func httpOnlyCalls(f packageFunc, model string) []string {
	params := serviceContextParams(f.decl, f.gstNames)
	if len(params) == 0 {
		return nil
	}
	var violations []string
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !params[svcErrDeclObj(ident)] || !slices.Contains(types.HTTPOnlyMethods, sel.Sel.Name) {
			return true
		}
		violations = append(violations, fmt.Sprintf("%s:%d: calls %s.%s, which only HTTP serves; the model %s is served over gRPC as well, so keep its services to what both transports provide",
			filepath.ToSlash(f.path), f.fset.Position(call.Pos()).Line, ident.Name, sel.Sel.Name, model))
		return true
	})
	return violations
}
