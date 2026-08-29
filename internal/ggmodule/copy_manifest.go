package ggmodule

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const moduleManifestFilename = "module.json"

type moduleManifest struct {
	Copy moduleCopyManifest `json:"copy"`
}

type moduleCopyManifest struct {
	// ExcludeSourceFiles lists framework-root relative source files that module
	// copy should skip, for example "internal/model/copytest/ignored.go". Excluded
	// files are not copied and do not participate in copy-time model/action
	// planning.
	ExcludeSourceFiles []string `json:"excludeSourceFiles"`
	// IncludeSourceFiles lists framework-root relative files under the module's
	// service source tree that copy must always carry as helper files, even when
	// no action service file references them. Modules declare source here that
	// serves no route and is reached only from project-owned assembly code, such
	// as a login second-factor verifier or a login observer.
	IncludeSourceFiles []string                       `json:"includeSourceFiles"`
	Middleware         []moduleCopyMiddlewareManifest `json:"middleware"`
	// RequiredAssembly lists the framework calls a copied module needs the
	// project to make, because copy reproduces routes, models and middleware
	// but not the rest of the module's Register body. Declaring a call here
	// makes gg check enforce it; postNotes stay for the wiring no check can
	// verify, such as writing an adapter type.
	RequiredAssembly []moduleCopyAssemblyManifest `json:"requiredAssembly"`
	PostNotes        []string                     `json:"postNotes"`
}

// moduleCopyAssemblyManifest declares one call the project must make for a
// copied module to work. Import is the full path of the package declaring the
// function, so the check resolves the call through each file's own import
// table and is not fooled by an alias.
type moduleCopyAssemblyManifest struct {
	Import   string `json:"import"`
	Function string `json:"function"`
	Reason   string `json:"reason"`
}

type moduleCopyMiddlewareScope string

const (
	moduleCopyMiddlewareScopeGlobal moduleCopyMiddlewareScope = "global"
	moduleCopyMiddlewareScopeAuth   moduleCopyMiddlewareScope = "auth"
)

type moduleCopyMiddlewareManifest struct {
	// SourceFile is framework-root relative and must point at a Go source file
	// in the framework middleware package. The target path is intentionally not
	// configurable: copied middleware becomes project-owned middleware with the
	// same filename under the project's middleware directory.
	SourceFile string `json:"sourceFile"`
	// Scope selects middleware.RegisterAuth ("auth") or middleware.Register
	// ("global") when module copy wires the handler into middleware/middleware.go.
	Scope moduleCopyMiddlewareScope `json:"scope"`
	// Handler is the zero-argument function in SourceFile that returns the
	// handler registered in middleware/middleware.go, for example CopyAuth.
	Handler string `json:"handler"`
}

func loadModuleManifest(moduleDir string) (moduleManifest, error) {
	path := filepath.Join(moduleDir, moduleManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return moduleManifest{}, fmt.Errorf("module copy requires %s: %w", path, err)
		}
		return moduleManifest{}, err
	}

	var manifest moduleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return moduleManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}

	manifest.Copy.PostNotes = cleanModuleCopyPostNotes(manifest.Copy.PostNotes)
	excludeSourceFiles, excludeErr := cleanModuleCopySourceFiles("excludeSourceFiles", manifest.Copy.ExcludeSourceFiles)
	if excludeErr != nil {
		return moduleManifest{}, fmt.Errorf("parse %s: %w", path, excludeErr)
	}
	manifest.Copy.ExcludeSourceFiles = excludeSourceFiles
	includeSourceFiles, includeErr := cleanModuleCopySourceFiles("includeSourceFiles", manifest.Copy.IncludeSourceFiles)
	if includeErr != nil {
		return moduleManifest{}, fmt.Errorf("parse %s: %w", path, includeErr)
	}
	manifest.Copy.IncludeSourceFiles = includeSourceFiles
	middleware, middlewareErr := cleanModuleCopyMiddleware(manifest.Copy.Middleware)
	if middlewareErr != nil {
		return moduleManifest{}, fmt.Errorf("parse %s: %w", path, middlewareErr)
	}
	manifest.Copy.Middleware = middleware
	assembly, assemblyErr := cleanModuleCopyAssembly(manifest.Copy.RequiredAssembly)
	if assemblyErr != nil {
		return moduleManifest{}, fmt.Errorf("parse %s: %w", path, assemblyErr)
	}
	manifest.Copy.RequiredAssembly = assembly
	return manifest, nil
}

func cleanModuleCopyPostNotes(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func cleanModuleCopySourceFiles(field string, values []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	for _, raw := range values {
		value, err := cleanModuleCopyRelativePath(raw)
		if err != nil {
			return nil, fmt.Errorf("%s contains unsafe framework-root relative path %q", field, raw)
		}
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

func cleanModuleCopyMiddleware(values []moduleCopyMiddlewareManifest) ([]moduleCopyMiddlewareManifest, error) {
	cleaned := make([]moduleCopyMiddlewareManifest, 0, len(values))
	for i, value := range values {
		sourceFile, err := cleanModuleCopyRelativePath(value.SourceFile)
		if err != nil || sourceFile == "" {
			return nil, fmt.Errorf("middleware[%d].sourceFile contains unsafe framework-root relative path %q", i, value.SourceFile)
		}
		// Keep middleware copy intentionally narrow: sources must come from the
		// framework middleware package and targets always land in the project
		// middleware package with the same filename. That avoids hidden copy-time
		// routing rules in copytest/register.go or arbitrary manifest target paths.
		if pathpkg.Dir(sourceFile) != "middleware" || !strings.HasSuffix(pathpkg.Base(sourceFile), ".go") || strings.HasSuffix(pathpkg.Base(sourceFile), "_test.go") {
			return nil, fmt.Errorf("middleware[%d].sourceFile must match middleware/*.go: %s", i, sourceFile)
		}

		scope := moduleCopyMiddlewareScope(strings.TrimSpace(string(value.Scope)))
		if scope != moduleCopyMiddlewareScopeGlobal && scope != moduleCopyMiddlewareScopeAuth {
			return nil, fmt.Errorf("middleware[%d].scope must be %q or %q: %q", i, moduleCopyMiddlewareScopeGlobal, moduleCopyMiddlewareScopeAuth, value.Scope)
		}

		handler := strings.TrimSpace(value.Handler)
		if !token.IsIdentifier(handler) {
			return nil, fmt.Errorf("middleware[%d].handler must be a Go identifier: %q", i, value.Handler)
		}

		cleaned = append(cleaned, moduleCopyMiddlewareManifest{
			SourceFile: sourceFile,
			Scope:      scope,
			Handler:    handler,
		})
	}
	return cleaned, nil
}

// cleanModuleCopyAssembly validates the required assembly calls. Every field is
// mandatory: an entry missing any of them would either fail to match anything
// or report a violation nobody can act on.
func cleanModuleCopyAssembly(values []moduleCopyAssemblyManifest) ([]moduleCopyAssemblyManifest, error) {
	cleaned := make([]moduleCopyAssemblyManifest, 0, len(values))
	for i, value := range values {
		importPath := strings.TrimSpace(value.Import)
		if importPath == "" {
			return nil, fmt.Errorf("requiredAssembly[%d].import must not be empty", i)
		}

		function := strings.TrimSpace(value.Function)
		if !token.IsIdentifier(function) || !token.IsExported(function) {
			return nil, fmt.Errorf("requiredAssembly[%d].function must be an exported Go identifier: %q", i, value.Function)
		}

		reason := strings.TrimSpace(value.Reason)
		if reason == "" {
			return nil, fmt.Errorf("requiredAssembly[%d].reason must not be empty", i)
		}

		cleaned = append(cleaned, moduleCopyAssemblyManifest{Import: importPath, Function: function, Reason: reason})
	}
	return cleaned, nil
}

func cleanModuleCopyRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "", nil
	}
	value = pathpkg.Clean(value)
	if value == "." || pathpkg.IsAbs(value) || value == ".." || strings.HasPrefix(value, "../") {
		return "", fmt.Errorf("unsafe framework-root relative path %q", value)
	}
	return value, nil
}
