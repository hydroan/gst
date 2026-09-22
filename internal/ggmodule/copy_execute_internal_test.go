package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

func TestCopyExecutionRequiresGenRunnerBeforeWritingFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	modelTarget := filepath.Join("model", "copytest", "copytest.go")
	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:       "copytest",
			ModelDir:   "model",
			ServiceDir: "service",
			Files: []moduleCopyFile{
				{
					Kind:       moduleCopyFileModel,
					TargetPath: modelTarget,
					Content:    []byte("package copytest\n"),
				},
			},
		},
	}

	err := exec.Run()
	if err == nil {
		t.Fatal("Run() succeeded, want an error when no gg gen runner is configured")
	}
	if len(exec.WrittenFiles) != 0 {
		t.Fatalf("Run() wrote %v before reporting the missing gg gen runner", exec.WrittenFiles)
	}
	if gghelper.FileExists(modelTarget) {
		t.Fatalf("Run() created %s before reporting the missing gg gen runner", modelTarget)
	}
}

func TestCopyExecutionPrunesStaleFilesBeforeGen(t *testing.T) {
	t.Chdir(t.TempDir())

	staleModel := filepath.Join("model", "copytest", "stale_model.go")
	staleService := filepath.Join("service", "copytest", "stale_service.go")
	for _, path := range []string{staleModel, staleService} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package copytest\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:              "copytest",
			ModelDir:          "model",
			ServiceDir:        "service",
			StaleModelFiles:   []string{staleModel},
			StaleServiceFiles: []string{staleService},
		},
		RunGen: func() error {
			// The prune must land before gg gen: a stale model file still
			// carries Design() DSL that gen would faithfully regenerate
			// registrations for.
			for _, path := range []string{staleModel, staleService} {
				if gghelper.FileExists(path) {
					t.Errorf("RunGen ran while stale file %s still exists", path)
				}
			}
			return nil
		},
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, path := range []string{staleModel, staleService} {
		if gghelper.FileExists(path) {
			t.Fatalf("Run() left stale file %s in place", path)
		}
	}
	wantDeleted := []string{staleModel, staleService}
	if !slices.Equal(exec.DeletedFiles, wantDeleted) {
		t.Fatalf("DeletedFiles = %v, want %v", exec.DeletedFiles, wantDeleted)
	}
}

// TestCopyExecutionReportsProgressThroughItsCallbacks pins what Run hands the
// command to print, in the order it happens: the title of each phase it
// enters and what it did to each file.
func TestCopyExecutionReportsProgressThroughItsCallbacks(t *testing.T) {
	t.Chdir(t.TempDir())

	model := filepath.Join("model", "copytest", "sample.go")
	stale := filepath.Join("service", "copytest", "stale.go")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("package copytest\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var events []string
	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:              "copytest",
			ModelDir:          "model",
			ServiceDir:        "service",
			Files:             []moduleCopyFile{{Kind: moduleCopyFileModel, TargetPath: model, Content: []byte("package copytest\n")}},
			StaleServiceFiles: []string{stale},
		},
		RunGen:    func() error { return nil },
		OnSection: func(title string) { events = append(events, "section "+title) },
		OnFile: func(status CopyWriteStatus, path string) {
			events = append(events, string(status)+" "+path)
		},
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{
		"section Copy Model Files",
		"CREATE " + model,
		"section Prune Stale Files",
		"DELETE " + stale,
		"section Copy Service Files",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("events = %q, want %q", events, want)
	}
}

func TestCopyExecutionPruneTreatsMissingStaleFileAsPruned(t *testing.T) {
	t.Chdir(t.TempDir())

	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:            "copytest",
			ModelDir:        "model",
			ServiceDir:      "service",
			StaleModelFiles: []string{filepath.Join("model", "copytest", "already_gone.go")},
		},
		RunGen: func() error { return nil },
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(exec.DeletedFiles) != 0 {
		t.Fatalf("DeletedFiles = %v, want empty for a file that was already gone", exec.DeletedFiles)
	}
}

func TestCopyExecutionPruneRejectsStalePathOutsideRoot(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "app")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(baseDir, "evil.go")
	if err := os.WriteFile(outsideFile, []byte("package evil\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:            "copytest",
			ModelDir:        "model",
			ServiceDir:      "service",
			StaleModelFiles: []string{filepath.Join("..", "evil.go")},
		},
		RunGen: func() error { return nil },
	}

	err := exec.Run()
	if err == nil {
		t.Fatal("Run() succeeded, want an error for a stale path outside the model dir")
	}
	if !gghelper.FileExists(outsideFile) {
		t.Fatal("Run() deleted a file outside the model dir")
	}
}

func TestModuleCopyRunPrunesStaleFilesAndKeepsExemptFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)

	targetModelDir := filepath.Join(projectDir, "model", "copytest")
	targetServiceDir := filepath.Join(projectDir, "service", "copytest")
	for _, dir := range []string{targetModelDir, targetServiceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(targetModelDir, "stale_model.go"), "package copytest\n")
	write(filepath.Join(targetModelDir, "stale_model_test.go"), "package copytest\n")
	write(filepath.Join(targetModelDir, "stale_cols.gen.go"), "// Code generated by gst; DO NOT EDIT.\n\npackage copytest\n")
	write(filepath.Join(targetServiceDir, "stale_service.go"), "package copytest\n")
	write(filepath.Join(targetServiceDir, "project_mock.go"), "// Code generated by MockGen. DO NOT EDIT.\n\npackage copytest\n")

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}
	exec := &CopyExecution{Plan: plan, RunGen: func() error { return nil }}
	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join("model", "copytest", "stale_model.go"),
		filepath.Join("service", "copytest", "stale_service.go"),
	} {
		if gghelper.FileExists(path) {
			t.Fatalf("Run() left stale file %s in place", path)
		}
	}
	for _, path := range []string{
		filepath.Join("model", "copytest", "stale_model_test.go"),
		filepath.Join("model", "copytest", "stale_cols.gen.go"),
		filepath.Join("service", "copytest", "project_mock.go"),
		filepath.Join("model", "copytest", "copytest.go"),
		filepath.Join("service", "copytest", "create.go"),
	} {
		if !gghelper.FileExists(path) {
			t.Fatalf("Run() should keep %s", path)
		}
	}
}
