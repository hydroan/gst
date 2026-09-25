package ggprune_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/ggprune"
)

// TestScanProtoFilesListsTheDefinitionsWhole pins that prune reads the pb
// directory whole: every .proto file under it is listed, one the project's
// Git ignore rules exclude and one below a directory named with a leading
// "_" included, other files are not, and a missing directory lists nothing.
func TestScanProtoFilesListsTheDefinitionsWhole(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeProjectFile(t, ".gitignore", "pb/record/item.proto\n")
	want := []string{
		filepath.Join(ggconst.DirPB, "_old", "draft.proto"),
		filepath.Join(ggconst.DirPB, "record.proto"),
		filepath.Join(ggconst.DirPB, "record", "item.proto"),
	}
	for _, path := range want {
		writeProjectFile(t, path, "syntax = \"proto3\";\n")
	}
	writeProjectFile(t, filepath.Join(ggconst.DirPB, "README.md"), "notes\n")

	got, err := ggprune.ScanProtoFiles(ggconst.DirPB)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("ScanProtoFiles() = %v, want %v", got, want)
	}

	none, err := ggprune.ScanProtoFiles(filepath.Join(projectDir, "missing"))
	if err != nil || len(none) != 0 {
		t.Fatalf("ScanProtoFiles(missing) = %v, %v; want nothing and no error", none, err)
	}
}

// TestPlanProtoFilesDeletesWhatGenWouldNotWrite pins that the plan deletes
// every existing definition gg gen would not write now, in the order they
// were scanned, keeps the ones a gst.yaml prune.ignore entry covers under
// Ignored, and reads the generated paths as gg gen hands them out,
// slash-separated.
func TestPlanProtoFilesDeletesWhatGenWouldNotWrite(t *testing.T) {
	current := filepath.Join(ggconst.DirPB, "record.proto")
	currentItem := filepath.Join(ggconst.DirPB, "record", "item.proto")
	stale := filepath.Join(ggconst.DirPB, "note.proto")
	kept := filepath.Join(ggconst.DirPB, "legacy", "item.proto")
	protect := ggconfig.PruneConfig{Ignore: []string{"pb/legacy"}}

	plan := ggprune.PlanProtoFiles([]string{current, stale, kept, currentItem}, []string{"pb/record.proto", "pb/record/item.proto"}, protect)

	if !slices.Equal(plan.Delete, []string{stale}) {
		t.Errorf("Delete = %v, want %v", plan.Delete, []string{stale})
	}
	if !slices.Equal(plan.Ignored, []string{kept}) {
		t.Errorf("Ignored = %v, want %v", plan.Ignored, []string{kept})
	}
}
