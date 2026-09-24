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
	"github.com/hydroan/gst/internal/ggprune"
)

// TestScanServiceFilesListsWhatIgnoreRulesCover pins that prune reads the
// service directory whole: a service file gg manages is listed when the
// project's Git ignore rules exclude it, and when it lies below testdata or a
// directory named with a leading "_", which the go command leaves out. Only
// gst.yaml's prune.ignore keeps a file from prune, and it is applied after the
// scan.
func TestScanServiceFilesListsWhatIgnoreRulesCover(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectFile(t, ".gitignore", "service/record/list.go\n")
	want := []string{
		filepath.Join(ggconst.DirService, "_old", "create.go"),
		filepath.Join(ggconst.DirService, "record", "create.go"),
		filepath.Join(ggconst.DirService, "record", "list.go"),
		filepath.Join(ggconst.DirService, "record", "testdata", "update.go"),
	}
	for _, path := range want {
		writeProjectFile(t, filepath.Join(projectDir, path), `package record

import "github.com/hydroan/gst/service"

type Creator struct {
	service.Base[*Record, *Record, *Record]
}
`)
	}

	files, err := ggprune.ScanServiceFiles(ggconst.DirService)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(files, want) {
		t.Fatalf("ScanServiceFiles() = %q, want %q", files, want)
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
	for _, dir := range []string{filepath.Join("service", "stale", "testdata"), filepath.Join("service", "placeholder")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeProjectFile(t, filepath.Join("service", "record", "list.go"), "package record\n")
	protect := ggconfig.PruneConfig{Ignore: []string{"service/placeholder"}}

	var removed []string
	ggprune.RemoveEmptyDirs("service", protect, func(dir string) {
		removed = append(removed, dir)
	})

	// The deepest directory goes first, which empties its parent in turn; a
	// testdata directory, which the go command leaves out, goes like any other.
	want := []string{filepath.Join("service", "stale", "testdata"), filepath.Join("service", "stale")}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed = %q, want %q", removed, want)
	}
	for _, kept := range []string{filepath.Join("service", "placeholder"), filepath.Join("service", "record")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s should be kept: %v", kept, err)
		}
	}
}
