package ggmodule

import (
	"encoding/json"
	"fmt"
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
	IgnoreFiles []string `json:"ignoreFiles"`
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
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		if value == "" {
			continue
		}
		value = pathpkg.Clean(value)
		if value == "." || pathpkg.IsAbs(value) || value == ".." || strings.HasPrefix(value, "../") {
			return nil, fmt.Errorf("ignoreFiles contains unsafe framework-root relative path %q", value)
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}
