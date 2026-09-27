package module_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// moduleManifestFilename is the copy contract each copyable module declares.
const moduleManifestFilename = "module.json"

// manifestKnownKeys are the "copy" keys this test validates. An unknown key
// fails the test on purpose: a manifest field nothing here checks would go
// back to being verified only by running a real copy.
var manifestKnownKeys = map[string]bool{
	"excludeSourceFiles": true,
	"includeSourceFiles": true,
	"middleware":         true,
	"interceptors":       true,
	"postNotes":          true,
	"requiredAssembly":   true,
}

// frameworkImportPrefix is the framework's own module path. A requiredAssembly
// entry naming a package under it resolves to a directory in this tree, so the
// function it demands can be checked for real.
const frameworkImportPrefix = "github.com/hydroan/gst/"

// moduleManifest mirrors the parts of module.json that name files and symbols.
type moduleManifest struct {
	Copy struct {
		ExcludeSourceFiles []string `json:"excludeSourceFiles"`
		IncludeSourceFiles []string `json:"includeSourceFiles"`
		Middleware         []struct {
			SourceFile string `json:"sourceFile"`
			Scope      string `json:"scope"`
			Handler    string `json:"handler"`
		} `json:"middleware"`
		Interceptors []struct {
			SourceFile string `json:"sourceFile"`
			Scope      string `json:"scope"`
			Handler    string `json:"handler"`
		} `json:"interceptors"`
		RequiredAssembly []struct {
			Import   string `json:"import"`
			Function string `json:"function"`
		} `json:"requiredAssembly"`
	} `json:"copy"`
}

// TestModuleManifestsMatchFrameworkTree pins every copyable module's manifest
// against the files it names. gg module copy resolves these paths and handlers
// at plan time, so a stale entry breaks a real copy while every unit test stays
// green — the copy suite deliberately runs on synthetic fixture modules so the
// tooling never depends on which business modules exist.
//
// The test lives here, beside the modules, for the same reason: deleting a
// module removes it from this walk instead of breaking a tooling test.
func TestModuleManifestsMatchFrameworkTree(t *testing.T) {
	frameworkRoot := ".."

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(entry.Name(), moduleManifestFilename)
		raw, readErr := os.ReadFile(manifestPath)
		if os.IsNotExist(readErr) {
			// Not every module is copyable; only a manifest makes it one.
			continue
		}
		require.NoError(t, readErr)
		checked++

		t.Run(entry.Name(), func(t *testing.T) {
			requireKnownManifestKeys(t, manifestPath, raw)

			var manifest moduleManifest
			require.NoError(t, json.Unmarshal(raw, &manifest), "%s is not valid JSON", manifestPath)

			for _, rel := range manifest.Copy.IncludeSourceFiles {
				requireFrameworkFile(t, frameworkRoot, manifestPath, "includeSourceFiles", rel)
			}
			for _, rel := range manifest.Copy.ExcludeSourceFiles {
				requireFrameworkFile(t, frameworkRoot, manifestPath, "excludeSourceFiles", rel)
			}
			for _, mw := range manifest.Copy.Middleware {
				path := requireFrameworkFile(t, frameworkRoot, manifestPath, "middleware.sourceFile", mw.SourceFile)
				requireExportedNiladicFunc(t, path, mw.Handler)
				requireCompilesAlone(t, frameworkRoot, path)
			}
			for _, ic := range manifest.Copy.Interceptors {
				path := requireFrameworkFile(t, frameworkRoot, manifestPath, "interceptors.sourceFile", ic.SourceFile)
				requireExportedNiladicFunc(t, path, ic.Handler)
				requireCompilesAlone(t, frameworkRoot, path)
			}
			for _, call := range manifest.Copy.RequiredAssembly {
				requireFrameworkPackageFunc(t, frameworkRoot, manifestPath, call.Import, call.Function)
			}
		})
	}
	require.Positive(t, checked, "no module manifest was found to validate")
}

// requireCompilesAlone pins that the file at path compiles on its own, the
// way gg module copy copies it into a project: as the one file of a package
// in a module of its own that requires the framework at frameworkRoot. A
// declaration the file takes from a sibling file of the framework's package
// fails here rather than in the project's build.
func requireCompilesAlone(t *testing.T, frameworkRoot, path string) {
	t.Helper()

	root, err := filepath.Abs(frameworkRoot)
	require.NoError(t, err)
	dir := t.TempDir()
	// The module path stays under the framework's, so the file may import
	// the framework's internal packages the way module sources do; the Go
	// version is the framework's own, read from its go.mod.
	frameworkMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	parsed, err := modfile.ParseLax("go.mod", frameworkMod, nil)
	require.NoError(t, err)
	goMod := "module github.com/hydroan/gst/copycheck\n\ngo " + parsed.Go.Version + "\n\nrequire github.com/hydroan/gst v0.0.0\n\nreplace github.com/hydroan/gst => " + root + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), goSum, 0o600))
	pkgDir := filepath.Join(dir, filepath.Base(filepath.Dir(path)))
	require.NoError(t, os.MkdirAll(pkgDir, 0o750))
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, filepath.Base(path)), source, 0o600))

	build := exec.Command("go", "build", "-o", os.DevNull, "./...")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s does not compile on its own:\n%s", path, out)
}

// requireKnownManifestKeys fails when the manifest carries a "copy" key this
// test does not validate.
func requireKnownManifestKeys(t *testing.T, manifestPath string, raw []byte) {
	t.Helper()

	var envelope struct {
		Copy map[string]json.RawMessage `json:"copy"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope), "%s is not valid JSON", manifestPath)
	for key := range envelope.Copy {
		require.True(t, manifestKnownKeys[key],
			"%s declares copy key %q that this test does not validate; extend manifestKnownKeys and check it",
			manifestPath, key)
	}
}

// requireFrameworkFile resolves a framework-root relative manifest entry and
// fails when it names no file. It returns the resolved path.
func requireFrameworkFile(t *testing.T, frameworkRoot, manifestPath, field, rel string) string {
	t.Helper()

	path := filepath.Join(frameworkRoot, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	require.NoErrorf(t, err, "%s %s names %q, which does not exist", manifestPath, field, rel)
	require.Falsef(t, info.IsDir(), "%s %s names directory %q, want a file", manifestPath, field, rel)

	return path
}

// requireExportedNiladicFunc fails unless the file declares name as a top-level
// exported function taking no arguments. Copy mode registers the handler as
// name() from the project's own middleware file, so any parameter would
// generate code that does not compile.
func requireExportedNiladicFunc(t *testing.T, path, name string) {
	t.Helper()

	require.NotEmpty(t, name, "%s declares no handler", path)
	require.True(t, ast.IsExported(name), "handler %s in %s must be exported", name, path)

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != name {
			continue
		}
		require.Empty(t, fn.Type.Params.List, "handler %s in %s must take no arguments", name, path)

		return
	}
	t.Fatalf("%s does not declare handler %s", path, name)
}

// requireFrameworkPackageFunc fails unless the framework package at importPath
// declares name as an exported function. gg check holds every project that
// copies the module to this call, so a renamed function would turn the check
// into a demand no project can satisfy.
func requireFrameworkPackageFunc(t *testing.T, frameworkRoot, manifestPath, importPath, name string) {
	t.Helper()

	require.True(t, strings.HasPrefix(importPath, frameworkImportPrefix),
		"%s requiredAssembly names %q, which is outside %s and cannot be checked here",
		manifestPath, importPath, frameworkImportPrefix)
	require.True(t, ast.IsExported(name), "%s requiredAssembly function %s must be exported", manifestPath, name)

	dir := filepath.Join(frameworkRoot, filepath.FromSlash(strings.TrimPrefix(importPath, frameworkImportPrefix)))
	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "%s requiredAssembly names package %q, which has no directory", manifestPath, importPath)

	for _, entry := range entries {
		name0 := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name0, ".go") || strings.HasSuffix(name0, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name0), nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == name {
				return
			}
		}
	}
	t.Fatalf("%s requiredAssembly names %s.%s, which the package does not declare", manifestPath, importPath, name)
}

// TestModuleHandlersMatchAcrossTransports pins that a module offering both a
// middleware and an interceptor offers them as one pair: the same handler
// names in the same order, in files of the same name, so what the HTTP
// listener gets under a name the gRPC listener gets under the same, from a
// file found at the same place.
func TestModuleHandlersMatchAcrossTransports(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	paired := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(entry.Name(), moduleManifestFilename)
		raw, readErr := os.ReadFile(manifestPath)
		if os.IsNotExist(readErr) {
			continue
		}
		require.NoError(t, readErr)
		var manifest moduleManifest
		require.NoError(t, json.Unmarshal(raw, &manifest), "%s is not valid JSON", manifestPath)
		if len(manifest.Copy.Middleware) == 0 || len(manifest.Copy.Interceptors) == 0 {
			continue
		}
		paired++
		t.Run(entry.Name(), func(t *testing.T) {
			require.Len(t, manifest.Copy.Interceptors, len(manifest.Copy.Middleware), "%s: one interceptor per middleware", manifestPath)
			for i, mw := range manifest.Copy.Middleware {
				ic := manifest.Copy.Interceptors[i]
				require.Equal(t, mw.Handler, ic.Handler, "%s: the interceptor of %s is named like it", manifestPath, mw.Handler)
				require.Equal(t, mw.Scope, ic.Scope, "%s: the interceptor of %s is registered like it", manifestPath, mw.Handler)
				require.Equal(t, filepath.Base(mw.SourceFile), filepath.Base(ic.SourceFile), "%s: the interceptor of %s lives in a file of the same name", manifestPath, mw.Handler)
			}
		})
	}
	require.Positive(t, paired, "the iam and authz modules offer both")
}
