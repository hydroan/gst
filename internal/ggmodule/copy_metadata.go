package ggmodule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const moduleCopyMetadataFilename = "module.copy.json"

type moduleCopyMetadata struct {
	PostCopyNotes []string `json:"postCopyNotes"`
}

func loadModuleCopyMetadata(moduleDir string) ([]string, error) {
	path := filepath.Join(moduleDir, moduleCopyMetadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metadata moduleCopyMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	notes := make([]string, 0, len(metadata.PostCopyNotes))
	for _, note := range metadata.PostCopyNotes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		notes = append(notes, note)
	}
	return notes, nil
}
