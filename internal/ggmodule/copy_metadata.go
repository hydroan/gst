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

const moduleCopyMetadataFilename = "module.copy.json"

type moduleCopyMetadata struct {
	PostCopyNotes []string `json:"postCopyNotes"`
	// IgnoreFiles lists framework-root relative source files that module copy
	// should skip, for example "internal/model/authz/button.go". Ignored files
	// are not copied and do not participate in copy-time model/action planning.
	IgnoreFiles []string                       `json:"ignoreFiles"`
	Middleware  []moduleCopyMiddlewareMetadata `json:"middleware"`
}

type moduleCopyMiddlewareMetadata struct {
	// Source is framework-root relative and must point at a Go source file in
	// the framework middleware package. The target path is intentionally not
	// configurable: copied middleware becomes project-owned middleware with the
	// same filename under the project's middleware directory.
	Source string `json:"source"`
	// Auth selects middleware.RegisterAuth instead of middleware.Register. This
	// mirrors the framework's two registration chains without introducing a
	// string enum that would need extra values before there is a real third mode.
	Auth bool `json:"auth"`
	// Function is the zero-argument function in Source that returns the handler
	// registered in middleware/middleware.go, for example Authz.
	Function string `json:"function"`
}

func loadModuleCopyMetadata(moduleDir string) (moduleCopyMetadata, error) {
	path := filepath.Join(moduleDir, moduleCopyMetadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return moduleCopyMetadata{}, nil
		}
		return moduleCopyMetadata{}, err
	}

	var metadata moduleCopyMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return moduleCopyMetadata{}, fmt.Errorf("parse %s: %w", path, err)
	}

	metadata.PostCopyNotes = cleanModuleCopyStrings(metadata.PostCopyNotes)
	ignoreFiles, ignoreErr := cleanModuleCopyIgnoreFiles(metadata.IgnoreFiles)
	if ignoreErr != nil {
		return moduleCopyMetadata{}, fmt.Errorf("parse %s: %w", path, ignoreErr)
	}
	metadata.IgnoreFiles = ignoreFiles
	middleware, middlewareErr := cleanModuleCopyMiddleware(metadata.Middleware)
	if middlewareErr != nil {
		return moduleCopyMetadata{}, fmt.Errorf("parse %s: %w", path, middlewareErr)
	}
	metadata.Middleware = middleware
	return metadata, nil
}

func cleanModuleCopyStrings(values []string) []string {
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

func cleanModuleCopyIgnoreFiles(values []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	for _, raw := range values {
		value, err := cleanModuleCopyRelativePath(raw)
		if err != nil {
			return nil, fmt.Errorf("ignoreFiles contains unsafe framework-root relative path %q", raw)
		}
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

func cleanModuleCopyMiddleware(values []moduleCopyMiddlewareMetadata) ([]moduleCopyMiddlewareMetadata, error) {
	cleaned := make([]moduleCopyMiddlewareMetadata, 0, len(values))
	for i, value := range values {
		source, err := cleanModuleCopyRelativePath(value.Source)
		if err != nil || source == "" {
			return nil, fmt.Errorf("middleware[%d].source contains unsafe framework-root relative path %q", i, value.Source)
		}
		// Keep middleware copy intentionally narrow: sources must come from the
		// framework middleware package and targets always land in the project
		// middleware package with the same filename. That avoids hidden copy-time
		// routing rules in authz/register.go or arbitrary manifest target paths.
		if pathpkg.Dir(source) != "middleware" || !strings.HasSuffix(pathpkg.Base(source), ".go") || strings.HasSuffix(pathpkg.Base(source), "_test.go") {
			return nil, fmt.Errorf("middleware[%d].source must match middleware/*.go: %s", i, source)
		}

		function := strings.TrimSpace(value.Function)
		if !token.IsIdentifier(function) {
			return nil, fmt.Errorf("middleware[%d].function must be a Go identifier: %q", i, value.Function)
		}

		cleaned = append(cleaned, moduleCopyMiddlewareMetadata{
			Source:   source,
			Auth:     value.Auth,
			Function: function,
		})
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
