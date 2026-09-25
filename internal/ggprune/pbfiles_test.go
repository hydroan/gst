package ggprune_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/ggprune"
)

// TestScanPBFilesListsTheGeneratedFilesWhole pins that prune reads the pb
// directory whole: every .proto and .pb.go file under it is listed, one the
// project's Git ignore rules exclude and one below a directory named with a
// leading "_" included, other files are not, and a missing directory lists
// nothing.
func TestScanPBFilesListsTheGeneratedFilesWhole(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectFile(t, ".gitignore", "pb/record/item.proto\n")
	want := []string{
		filepath.Join(ggconst.DirPB, "_old", "draft.proto"),
		filepath.Join(ggconst.DirPB, "record.pb.go"),
		filepath.Join(ggconst.DirPB, "record.proto"),
		filepath.Join(ggconst.DirPB, "record", "item.proto"),
		filepath.Join(ggconst.DirPB, "record_grpc.pb.go"),
	}
	for _, path := range want {
		writeProjectFile(t, path, "generated\n")
	}
	writeProjectFile(t, filepath.Join(ggconst.DirPB, "README.md"), "notes\n")

	got, err := ggprune.ScanPBFiles(ggconst.DirPB)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("ScanPBFiles() = %v, want %v", got, want)
	}

	none, err := ggprune.ScanPBFiles(filepath.Join(projectDir, "missing"))
	if err != nil || len(none) != 0 {
		t.Fatalf("ScanPBFiles(missing) = %v, %v; want nothing and no error", none, err)
	}
}

// TestPlanPBFilesDeletesWhatGenWouldNotWrite pins that the plan deletes
// every existing file gg gen would not write now, definitions and the Go
// files compiled from them alike, in the order they were scanned, keeps the
// ones a gst.yaml prune.ignore entry covers under Ignored, and reads the
// generated paths as gg gen hands them out, slash-separated.
func TestPlanPBFilesDeletesWhatGenWouldNotWrite(t *testing.T) {
	current := filepath.Join(ggconst.DirPB, "record.proto")
	currentGo := filepath.Join(ggconst.DirPB, "record.pb.go")
	currentItem := filepath.Join(ggconst.DirPB, "record", "item.proto")
	stale := filepath.Join(ggconst.DirPB, "note.proto")
	staleGo := filepath.Join(ggconst.DirPB, "note_grpc.pb.go")
	kept := filepath.Join(ggconst.DirPB, "legacy", "item.proto")
	protect := ggconfig.PruneConfig{Ignore: []string{"pb/legacy"}}

	plan := ggprune.PlanPBFiles([]string{current, stale, kept, currentItem, currentGo, staleGo}, []string{"pb/record.proto", "pb/record/item.proto", "pb/record.pb.go"}, protect)

	if !slices.Equal(plan.Delete, []string{stale, staleGo}) {
		t.Errorf("Delete = %v, want %v", plan.Delete, []string{stale, staleGo})
	}
	if !slices.Equal(plan.Ignored, []string{kept}) {
		t.Errorf("Ignored = %v, want %v", plan.Ignored, []string{kept})
	}
}
