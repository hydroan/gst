package module

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// middlewareImportPath is the framework middleware package. Module registration
// calls are matched through it rather than by the qualifier spelling, so an
// aliased import is read correctly.
const middlewareImportPath = "github.com/hydroan/gst/middleware"

// copyableModuleManifests returns every module that declares a copy manifest,
// keyed by module name. Only copyable modules are subject to the rules here:
// an add-only module is always linked through the Register call the project
// writes, so none of the copy-path hazards apply to it.
func copyableModuleManifests(t *testing.T) map[string]moduleManifest {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	manifests := make(map[string]moduleManifest)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(entry.Name(), moduleManifestFilename))
		if os.IsNotExist(readErr) {
			continue
		}
		require.NoError(t, readErr)

		var manifest moduleManifest
		require.NoError(t, json.Unmarshal(raw, &manifest))
		manifests[entry.Name()] = manifest
	}
	require.NotEmpty(t, manifests, "no copyable module was found to validate")

	return manifests
}

// parseModulePackage parses the non-test sources of one directory.
func parseModulePackage(t *testing.T, dir string) []*ast.File {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	files := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		files = append(files, file)
	}

	return files
}

// TestCopyableModuleServicesInstallNoHookFromInit pins the assembly rule that a
// copied module's service package must not install framework hooks from init.
//
// Package init only runs when the package is linked, and on the copy path the
// only import of service/<name> comes from the generated route registrations —
// which a project removes by ignoring the module's routes in gst.yaml. A hook
// installed from init therefore survives or disappears according to unrelated
// route configuration, silently and without any build error.
func TestCopyableModuleServicesInstallNoHookFromInit(t *testing.T) {
	for name := range copyableModuleManifests(t) {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("..", "internal", "service", name)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return
			}
			for _, file := range parseModulePackage(t, dir) {
				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					require.Falsef(t, ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == "init",
						"internal/service/%s declares init(): export the hook implementation and install it from Register instead", name)
				}
			}
		})
	}
}

// TestModuleMiddlewareRegistrationsMatchManifest pins add/copy parity for
// middleware: the add path mounts what Register calls, the copy path mounts
// what module.json declares, and a project must end up with the same handlers
// either way.
//
// Both directions are checked. A registration missing from the manifest is
// add-only and silently absent after a copy; a manifest entry nothing registers
// is copy-only and silently absent after an add.
func TestModuleMiddlewareRegistrationsMatchManifest(t *testing.T) {
	for name, manifest := range copyableModuleManifests(t) {
		t.Run(name, func(t *testing.T) {
			declared := make([]string, 0, len(manifest.Copy.Middleware))
			for _, mw := range manifest.Copy.Middleware {
				declared = append(declared, mw.Scope+":"+mw.Handler)
			}

			registered := make([]string, 0, len(declared))
			for _, file := range parseModulePackage(t, name) {
				registered = append(registered, middlewareRegistrations(t, name, file)...)
			}

			sort.Strings(declared)
			sort.Strings(registered)
			require.Equal(t, declared, registered,
				"module %s registers %v but module.json declares %v; add and copy must mount the same middleware",
				name, registered, declared)
		})
	}
}

// middlewareRegistrations returns the "<scope>:<handler>" of every middleware
// registration the file makes.
func middlewareRegistrations(t *testing.T, module string, file *ast.File) []string {
	t.Helper()

	imports := importPathsByLocalName(file)
	found := make([]string, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		scope, ok := middlewareRegisterScope(call, imports)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			handler, handlerOK := middlewareHandlerName(arg, imports)
			require.Truef(t, handlerOK,
				"module %s registers middleware from an expression that is not a %s handler call; "+
					"the implementation must live in the framework middleware package so gg module copy can carry it",
				module, middlewareImportPath)
			found = append(found, scope+":"+handler)
		}

		return true
	})

	return found
}

// middlewareRegisterScope reports the manifest scope a middleware registration
// call corresponds to.
func middlewareRegisterScope(call *ast.CallExpr, imports map[string]string) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return "", false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || imports[qualifier.Name] != middlewareImportPath {
		return "", false
	}
	switch selector.Sel.Name {
	case "Register":
		return "global", true
	case "RegisterAuth":
		return "auth", true
	default:
		return "", false
	}
}

// middlewareHandlerName returns the handler name of a middleware.Xxx() argument.
func middlewareHandlerName(arg ast.Expr, imports map[string]string) (string, bool) {
	call, ok := arg.(*ast.CallExpr)
	if !ok || len(call.Args) > 0 {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return "", false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || imports[qualifier.Name] != middlewareImportPath {
		return "", false
	}

	return selector.Sel.Name, true
}

// importPathsByLocalName maps each import's local name to its path.
func importPathsByLocalName(file *ast.File) map[string]string {
	paths := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		if spec.Path == nil {
			continue
		}
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path
		if index := strings.LastIndex(path, "/"); index >= 0 {
			name = path[index+1:]
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		paths[name] = path
	}

	return paths
}
