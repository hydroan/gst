package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// TestScanExistingServiceFilesSkipsGitIgnoredFiles pins that prune never
// considers a service file the project's Git ignore rules exclude: gg check
// leaves such a file alone, so prune must not delete it either.
func TestScanExistingServiceFilesSkipsGitIgnoredFiles(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectFile(t, ".gitignore", "service/record/list.go\n")
	kept := filepath.Join(ggconst.DirService, "record", "create.go")
	ignored := filepath.Join(ggconst.DirService, "record", "list.go")
	for _, path := range []string{kept, ignored} {
		writeProjectFile(t, filepath.Join(projectDir, path), `package record

import "github.com/hydroan/gst/service"

type Creator struct {
	service.Base[*Record, *Record, *Record]
}
`)
	}

	files := scanExistingServiceFiles(ggconst.DirService, gghelper.NewProjectIgnore())

	if slices.Contains(files, ignored) {
		t.Fatalf("scanExistingServiceFiles() = %q, want the Git ignored file left out", files)
	}
	if !slices.Contains(files, kept) {
		t.Fatalf("scanExistingServiceFiles() = %q, want it to hold %q", files, kept)
	}
}
