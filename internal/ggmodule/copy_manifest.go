package ggmodule

import (
	"encoding/json"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
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
	IncludeSourceFiles []string                    `json:"includeSourceFiles"`
	Middleware         []moduleCopyHandlerManifest `json:"middleware"`
	// Interceptors are the module's gRPC interceptors, declared like its
	// middleware and copied into the project's interceptor directory, into a
	// project serving gRPC alone: one with a model declaring GRPC().
	Interceptors []moduleCopyHandlerManifest `json:"interceptors"`
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

type moduleCopyHandlerScope string

const (
	moduleCopyHandlerScopeGlobal moduleCopyHandlerScope = "global"
	moduleCopyHandlerScopeAuth   moduleCopyHandlerScope = "auth"
)

type moduleCopyHandlerManifest struct {
	// SourceFile is framework-root relative and must point at a Go source file
	// in the framework middleware or interceptor package. The target path is
	// intentionally not configurable: a copied handler becomes a project-owned
	// file with the same name under the project's directory of that kind.
	SourceFile string `json:"sourceFile"`
	// Scope selects RegisterAuth ("auth") or Register ("global") of the
	// framework package of the directory when module copy wires the handler
	// into its registration file.
	Scope moduleCopyHandlerScope `json:"scope"`
	// Handler is the zero-argument function in SourceFile that returns the
	// handler registered in middleware/middleware.go, for example CopyAuth.
	Handler string `json:"handler"`
}

func loadModuleManifest(moduleDir string) (moduleManifest, error) {
	manifestPath := filepath.Join(moduleDir, moduleManifestFilename)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return moduleManifest{}, errors.Wrapf(err, "module copy requires %s", manifestPath)
		}
		return moduleManifest{}, err
	}

	var manifest moduleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return moduleManifest{}, errors.Wrapf(err, "parse %s", manifestPath)
	}

	manifest.Copy.PostNotes = cleanModuleCopyPostNotes(manifest.Copy.PostNotes)
	excludeSourceFiles, excludeErr := cleanModuleCopySourceFiles("excludeSourceFiles", manifest.Copy.ExcludeSourceFiles)
	if excludeErr != nil {
		return moduleManifest{}, errors.Wrapf(excludeErr, "parse %s", manifestPath)
	}
	manifest.Copy.ExcludeSourceFiles = excludeSourceFiles
	includeSourceFiles, includeErr := cleanModuleCopySourceFiles("includeSourceFiles", manifest.Copy.IncludeSourceFiles)
	if includeErr != nil {
		return moduleManifest{}, errors.Wrapf(includeErr, "parse %s", manifestPath)
	}
	manifest.Copy.IncludeSourceFiles = includeSourceFiles
	middleware, middlewareErr := cleanModuleCopyHandlers("middleware", manifest.Copy.Middleware)
	if middlewareErr != nil {
		return moduleManifest{}, errors.Wrapf(middlewareErr, "parse %s", manifestPath)
	}
	manifest.Copy.Middleware = middleware
	interceptors, interceptorsErr := cleanModuleCopyHandlers("interceptors", manifest.Copy.Interceptors)
	if interceptorsErr != nil {
		return moduleManifest{}, errors.Wrapf(interceptorsErr, "parse %s", manifestPath)
	}
	manifest.Copy.Interceptors = interceptors
	assembly, assemblyErr := cleanModuleCopyAssembly(manifest.Copy.RequiredAssembly)
	if assemblyErr != nil {
		return moduleManifest{}, errors.Wrapf(assemblyErr, "parse %s", manifestPath)
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
			return nil, errors.Newf("%s contains unsafe framework-root relative path %q", field, raw)
		}
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

// cleanModuleCopyHandlers validates the handler entries of the manifest
// field named field, "middleware" or "interceptors", whose source files
// must lie in the framework directory of the same kind: middleware/*.go for
// the middleware, interceptor/*.go for the interceptors.
func cleanModuleCopyHandlers(field string, values []moduleCopyHandlerManifest) ([]moduleCopyHandlerManifest, error) {
	sourceDir := map[string]string{"middleware": middlewareManagedDir.pkg, "interceptors": interceptorManagedDir.pkg}[field]
	cleaned := make([]moduleCopyHandlerManifest, 0, len(values))
	for i, value := range values {
		sourceFile, err := cleanModuleCopyRelativePath(value.SourceFile)
		if err != nil || sourceFile == "" {
			return nil, errors.Newf("%s[%d].sourceFile contains unsafe framework-root relative path %q", field, i, value.SourceFile)
		}
		// Keep the copy intentionally narrow: sources must come from the
		// framework package of the kind and targets always land in the
		// project package of the same name with the same filename. That
		// avoids hidden copy-time routing rules in copytest/register.go or
		// arbitrary manifest target paths.
		if path.Dir(sourceFile) != sourceDir || !strings.HasSuffix(path.Base(sourceFile), ".go") || strings.HasSuffix(path.Base(sourceFile), "_test.go") {
			return nil, errors.Newf("%s[%d].sourceFile must match %s/*.go: %s", field, i, sourceDir, sourceFile)
		}

		scope := moduleCopyHandlerScope(strings.TrimSpace(string(value.Scope)))
		if scope != moduleCopyHandlerScopeGlobal && scope != moduleCopyHandlerScopeAuth {
			return nil, errors.Newf("%s[%d].scope must be %q or %q: %q", field, i, moduleCopyHandlerScopeGlobal, moduleCopyHandlerScopeAuth, value.Scope)
		}

		handler := strings.TrimSpace(value.Handler)
		if !token.IsIdentifier(handler) {
			return nil, errors.Newf("%s[%d].handler must be a Go identifier: %q", field, i, value.Handler)
		}

		cleaned = append(cleaned, moduleCopyHandlerManifest{
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
			return nil, errors.Newf("requiredAssembly[%d].import must not be empty", i)
		}

		function := strings.TrimSpace(value.Function)
		if !token.IsIdentifier(function) || !token.IsExported(function) {
			return nil, errors.Newf("requiredAssembly[%d].function must be an exported Go identifier: %q", i, value.Function)
		}

		reason := strings.TrimSpace(value.Reason)
		if reason == "" {
			return nil, errors.Newf("requiredAssembly[%d].reason must not be empty", i)
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
	value = path.Clean(value)
	if value == "." || path.IsAbs(value) || value == ".." || strings.HasPrefix(value, "../") {
		return "", errors.Newf("unsafe framework-root relative path %q", value)
	}
	return value, nil
}
