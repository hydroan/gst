package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestScanOrphanServiceDirsReportsUnmanagedFilesWhenModelMissing(t *testing.T) {
	withTempCodegenDirs(t, func() {
		orphanFile := filepath.Join(serviceDir, "mfa", "note.txt")
		writeTestFile(t, orphanFile, "manual note")

		orphans := scanOrphanServiceDirs(currentServiceDirs(nil), nil)

		if len(orphans) != 1 {
			t.Fatalf("expected one orphan service dir, got %d: %#v", len(orphans), orphans)
		}
		if orphans[0].Path != filepath.Join(serviceDir, "mfa") {
			t.Fatalf("expected service/mfa orphan, got %s", orphans[0].Path)
		}
		if len(orphans[0].Files) != 1 || orphans[0].Files[0] != orphanFile {
			t.Fatalf("expected only unmanaged file %s, got %#v", orphanFile, orphans[0].Files)
		}
	})
}

func TestCleanOrphanServiceDirsDeletesOnlyUnmanagedFiles(t *testing.T) {
	withTempCodegenDirs(t, func() {
		orphanFile := filepath.Join(serviceDir, "mfa", "note.txt")
		managedFile := filepath.Join(serviceDir, "mfa", "create.go")
		writeTestFile(t, orphanFile, "manual note")
		writeTestFile(t, managedFile, "package mfa\n")

		orphans := scanOrphanServiceDirs(currentServiceDirs(nil), nil)
		cleanOrphanServiceDirs(orphans)

		if _, err := os.Stat(orphanFile); !os.IsNotExist(err) {
			t.Fatalf("expected unmanaged file to be deleted, stat err: %v", err)
		}
		if _, err := os.Stat(managedFile); err != nil {
			t.Fatalf("expected managed service file to be kept, stat err: %v", err)
		}
	})
}

func TestScanOrphanServiceDirsIgnoresChildrenUnderCurrentModelServiceDir(t *testing.T) {
	withTempCodegenDirs(t, func() {
		manualFile := filepath.Join(serviceDir, "mfa", "helper", "note.txt")
		writeTestFile(t, manualFile, "manual helper")

		models := []*gen.ModelInfo{
			{ModelFilePath: filepath.Join(modelDir, "mfa", "mfa.go")},
		}

		orphans := scanOrphanServiceDirs(currentServiceDirs(models), nil)
		if len(orphans) != 0 {
			t.Fatalf("expected no orphan dirs under current model service dir, got %#v", orphans)
		}
	})
}

func TestScanOrphanServiceDirsReportsSiblingUnderAncestorOnly(t *testing.T) {
	withTempCodegenDirs(t, func() {
		orphanFile := filepath.Join(serviceDir, "config", "old", "note.txt")
		writeTestFile(t, orphanFile, "manual old service")

		models := []*gen.ModelInfo{
			{ModelFilePath: filepath.Join(modelDir, "config", "namespace.go")},
		}

		orphans := scanOrphanServiceDirs(currentServiceDirs(models), nil)
		if len(orphans) != 1 {
			t.Fatalf("expected one orphan service dir, got %d: %#v", len(orphans), orphans)
		}
		if orphans[0].Path != filepath.Join(serviceDir, "config", "old") {
			t.Fatalf("expected service/config/old orphan, got %s", orphans[0].Path)
		}
	})
}

func TestScanOrphanServiceDirsHonorsIgnorePatterns(t *testing.T) {
	withTempCodegenDirs(t, func() {
		ignoredDir := filepath.Join(serviceDir, "manual")
		writeTestFile(t, filepath.Join(ignoredDir, "note.txt"), "manual service")

		orphans := scanOrphanServiceDirs(currentServiceDirs(nil), []string{ignoredDir})
		if len(orphans) != 0 {
			t.Fatalf("expected ignored orphan dir to be skipped, got %#v", orphans)
		}
	})
}

func TestConfirmCleanOrphansRequiresExactPhrase(t *testing.T) {
	withStdin(t, "delete orphan service leftovers\n", func() {
		if !confirmCleanOrphanServiceDirs() {
			t.Fatal("expected exact confirmation phrase to be accepted")
		}
	})

	withStdin(t, "y\n", func() {
		if confirmCleanOrphanServiceDirs() {
			t.Fatal("expected shorthand confirmation to be rejected")
		}
	})
}

func withTempCodegenDirs(t *testing.T, run func()) {
	t.Helper()

	oldModelDir := modelDir
	oldServiceDir := serviceDir
	modelDir = filepath.Join(t.TempDir(), "model")
	serviceDir = filepath.Join(t.TempDir(), "service")
	t.Cleanup(func() {
		modelDir = oldModelDir
		serviceDir = oldServiceDir
	})

	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		t.Fatalf("create model dir: %v", err)
	}
	if err := os.MkdirAll(serviceDir, 0o700); err != nil {
		t.Fatalf("create service dir: %v", err)
	}

	run()
}

func withStdin(t *testing.T, input string, run func()) {
	t.Helper()

	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = oldStdin
		if err := reader.Close(); err != nil {
			t.Fatalf("close stdin reader: %v", err)
		}
	})

	run()
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
