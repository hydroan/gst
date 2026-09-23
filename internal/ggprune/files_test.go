package ggprune_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggprune"
)

// TestScanServiceFilesSkipsGitIgnoredFiles pins that prune never
// considers a service file the project's Git ignore rules exclude: gg check
// leaves such a file alone, so prune must not delete it either.
func TestScanServiceFilesSkipsGitIgnoredFiles(t *testing.T) {
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

	files, err := ggprune.ScanServiceFiles(ggconst.DirService, gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}

	if slices.Contains(files, ignored) {
		t.Fatalf("ScanServiceFiles() = %q, want the Git ignored file left out", files)
	}
	if !slices.Contains(files, kept) {
		t.Fatalf("ScanServiceFiles() = %q, want it to hold %q", files, kept)
	}
}

func TestPlanFiles(t *testing.T) {
	current := filepath.Join("service", "authz", "role.go")
	disabled := filepath.Join("service", "authz", "list.go")
	kept := filepath.Join("service", "signup", "create.go")
	protected := filepath.Join("service", "legacy", "create.go")
	protect := ggconfig.PruneConfig{Ignore: []string{"service/legacy"}}

	plan := ggprune.PlanFiles([]string{current, disabled, kept, protected}, []*gen.ModelInfo{orphanPruneModel()}, map[string]bool{kept: true}, protect)

	if !slices.Equal(plan.Delete, []string{disabled}) {
		t.Fatalf("Delete = %q, want only the file no enabled action expects", plan.Delete)
	}
	if !slices.Equal(plan.Ignored, []string{protected}) {
		t.Fatalf("Ignored = %q, want the file prune.ignore covers", plan.Ignored)
	}
}

func TestRemoveFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	present := filepath.Join("service", "record", "list.go")
	missing := filepath.Join("service", "record", "get.go")
	writeProjectFile(t, present, "package record\n")

	var reported []string
	ggprune.RemoveFiles([]string{present, missing}, func(path string, err error) {
		reported = append(reported, fmt.Sprintf("%s deleted=%t", path, err == nil))
	})

	// A failure is reported and the rest still run, in order.
	want := []string{present + " deleted=true", missing + " deleted=false"}
	if !slices.Equal(reported, want) {
		t.Fatalf("reported = %q, want %q", reported, want)
	}
	if _, err := os.Stat(present); !os.IsNotExist(err) {
		t.Fatalf("%s should be deleted, stat error = %v", present, err)
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{filepath.Join("service", "stale", "nested"), filepath.Join("service", "placeholder")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeProjectFile(t, filepath.Join("service", "record", "list.go"), "package record\n")
	protect := ggconfig.PruneConfig{Ignore: []string{"service/placeholder"}}

	var removed []string
	ggprune.RemoveEmptyDirs("service", protect, gghelper.NewProjectIgnore(), func(dir string) {
		removed = append(removed, dir)
	})

	// The deepest directory goes first, which empties its parent in turn.
	want := []string{filepath.Join("service", "stale", "nested"), filepath.Join("service", "stale")}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed = %q, want %q", removed, want)
	}
	for _, kept := range []string{filepath.Join("service", "placeholder"), filepath.Join("service", "record")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s should be kept: %v", kept, err)
		}
	}
}
