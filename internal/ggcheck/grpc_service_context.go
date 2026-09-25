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

	"github.com/hydroan/gst/dsl"
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
// (see modelinfo.ServiceTarget), read whole, helpers included, since a
// service method reaches them; left alone are the service files of the
// actions gRPC does not serve (see dsl.HTTPOnlyAction), an SSE service
// calling ctx.SSE as it must, and those of models served over HTTP alone
// that share the package. The analysis is syntactic like the other checks:
// it follows the parameter object, so a local variable of the same name is
// not mistaken for it, and it does not follow the context into a variable
// assigned from it.
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
		if m.Design == nil || !m.Design.Enabled || ignore.Ignores(m.ModelFilePath, false) {
			continue
		}
		m.Design.Range(func(_ string, act *dsl.Action) {
			if !act.Service {
				return
			}
			target := modelinfo.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			served[target.FilePath] = m.Design.GRPC && !dsl.HTTPOnlyAction(act.Phase.MethodName())
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
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			path := filepath.Join(dir, name)
			if entry.IsDir() || !strings.HasSuffix(name, ggconst.ExtensionGo) || strings.HasSuffix(name, ggconst.PatternTestFile) || ignore.Ignores(path, false) {
				continue
			}
			if grpc, action := served[path]; action && !grpc {
				continue
			}
			violations = append(violations, httpOnlyCalls(path, owners[dir])...)
		}
	}
	return violations
}

// httpOnlyCalls lists the calls of the HTTP-only methods in the file at
// path, each as a violation naming the model that owns the package, in
// source order. A file that fails to parse is reported as one violation.
func httpOnlyCalls(path, model string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	gstNames := goast.ImportedNames(file, gstImportPath, "gst")
	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		params := serviceContextParams(fn, gstNames)
		if len(params) == 0 {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
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
				filepath.ToSlash(path), fset.Position(call.Pos()).Line, ident.Name, sel.Sel.Name, model))
			return true
		})
	}
	return violations
}
