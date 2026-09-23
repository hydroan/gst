package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// TestPruneServiceFilesKeepsWhatPruneIgnoreCovers pins that prune never
// deletes what a gst.yaml prune.ignore entry covers: a disabled service file,
// a whole directory of them, or a directory left empty. An entry naming
// nothing on disk is warned about.
func TestPruneServiceFilesKeepsWhatPruneIgnoreCovers(t *testing.T) {
	t.Chdir(t.TempDir())
	listFile := filepath.Join(ggconst.DirService, "record", "list.go")
	legacyFile := filepath.Join(ggconst.DirService, "legacy", "create.go")
	writeProjectFile(t, listFile, "package record\n")
	writeProjectFile(t, legacyFile, "package legacy\n")
	keptEmpty := filepath.Join(ggconst.DirService, "placeholder")
	removedEmpty := filepath.Join(ggconst.DirService, "stale")
	for _, dir := range []string{keptEmpty, removedEmpty} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	protect := ggconfig.PruneConfig{Ignore: []string{"service/record/list.go", "service/legacy", "service/placeholder", "service/gone"}}

	// No model declares either file, so both are disabled; with every one of
	// them covered, prune has nothing to ask about. Were one left uncovered,
	// the yes waiting on stdin would delete it.
	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			pruneServiceFiles([]string{listFile, legacyFile}, nil, nil, nil, protect, gghelper.NewProjectIgnore())
		})
	})

	for _, path := range []string{listFile, legacyFile, keptEmpty} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should survive prune: %v", path, err)
		}
	}
	if _, err := os.Stat(removedEmpty); !os.IsNotExist(err) {
		t.Errorf("%s is empty and not covered, want it removed; stat error = %v", removedEmpty, err)
	}
	if !strings.Contains(stdout, `gst.yaml prune.ignore entry "service/gone" names no file or directory`) {
		t.Errorf("output lacks the warning about the entry naming nothing:\n%s", stdout)
	}
}

// TestPruneServiceFilesRemindsOfUnreadSettingsBeforeAsking pins that with an
// old .gg.yaml next to gst.yaml, prune says right before asking to delete that
// the paths it lists are not protected.
func TestPruneServiceFilesRemindsOfUnreadSettingsBeforeAsking(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, ".gg.yaml", "prune:\n  ignore:\n    - service/record\n")
	listFile := filepath.Join(ggconst.DirService, "record", "list.go")
	writeProjectFile(t, listFile, "package record\n")

	var stdout string
	withStdin(t, "n\n", func() {
		stdout = captureStdout(t, func() {
			pruneServiceFiles([]string{listFile}, nil, nil, nil, ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())
		})
	})

	reminder := strings.Index(stdout, ".gg.yaml is not read, so the paths it lists are not protected here")
	prompt := strings.Index(stdout, "Do you want to delete these files?")
	if reminder < 0 || prompt < 0 || reminder > prompt {
		t.Fatalf("want the reminder right before the prompt, got:\n%s", stdout)
	}
	if _, err := os.Stat(listFile); err != nil {
		t.Fatalf("answering no must keep %s: %v", listFile, err)
	}
}

// TestPruneRunStopsOnABrokenConfig pins that gg prune reports what stops it
// before it deletes anything, here a gst.yaml prune.ignore entry outside
// service/ and middleware/, as an error the command prints, not as a panic.
func TestPruneRunStopsOnABrokenConfig(t *testing.T) {
	newGenProject(t)
	listFile := filepath.Join(ggconst.DirService, "record", "list.go")
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "record.go"), "package model\n")
	writeProjectFile(t, listFile, "package record\n")
	writeProjectFile(t, ggconfig.FileName, "version: 1\nprune:\n  ignore:\n    - model/record.go\n")

	err := pruneRun()

	if err == nil || !strings.Contains(err.Error(), `entry "model/record.go" is outside service/`) {
		t.Fatalf("pruneRun() error = %v, want the prune.ignore entry outside service/ reported", err)
	}
	if _, statErr := os.Stat(listFile); statErr != nil {
		t.Fatalf("a run that stops must delete nothing: %v", statErr)
	}
}

// TestPruneServiceFilesCleansUpAfterARemovedCopiedModule pins that the
// removal path gg module copy prints, deleting model/<name> and then pruning
// with --clean-orphans, takes the middleware the copy wrote as well: the file
// carrying the module's ownership marker, its register calls, and the service
// directory only that middleware imported. Without --clean-orphans they are
// listed and kept, and a prune.ignore entry keeps the file for good, together
// with what it imports.
func TestPruneServiceFilesCleansUpAfterARemovedCopiedModule(t *testing.T) {
	t.Run("cleaned with --clean-orphans", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t, true)

		withStdin(t, cleanOrphansConfirmation+"\n", func() {
			captureStdout(t, func() {
				pruneServiceFiles(nil, nil, nil, nil, ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())
			})
		})

		for _, path := range []string{middlewareFile, helperFile} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s should be deleted with the module, stat error = %v", path, err)
			}
		}
		registration, err := os.ReadFile(registrationFile)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(registration), "SampleAuth") || strings.Contains(string(registration), `"github.com/hydroan/gst/middleware"`) {
			t.Errorf("%s still registers the deleted middleware:\n%s", registrationFile, registration)
		}
	})

	t.Run("listed without --clean-orphans", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t, false)

		stdout := captureStdout(t, func() {
			pruneServiceFiles(nil, nil, nil, nil, ggconfig.PruneConfig{}, gghelper.NewProjectIgnore())
		})

		for _, path := range []string{middlewareFile, registrationFile, helperFile} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s should be kept without --clean-orphans: %v", path, err)
			}
		}
		for _, want := range []string{"Orphan Module Middleware Files Kept", middlewareFile + " (copied with module sample, whose model/sample is gone"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("output lacks %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("kept by prune.ignore", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t, true)
		protect := ggconfig.PruneConfig{Ignore: []string{filepath.ToSlash(middlewareFile)}}

		withStdin(t, cleanOrphansConfirmation+"\n", func() {
			captureStdout(t, func() {
				pruneServiceFiles(nil, nil, nil, nil, protect, gghelper.NewProjectIgnore())
			})
		})

		for _, path := range []string{middlewareFile, registrationFile, helperFile} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s should be kept: %v", path, err)
			}
		}
		registration, err := os.ReadFile(registrationFile)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(registration), "middleware.RegisterAuth(SampleAuth())") {
			t.Errorf("%s lost the register call of the kept middleware:\n%s", registrationFile, registration)
		}
	})
}

// setupRemovedModuleProject moves the test into a project, module tmpapp, that
// copied the module sample and then deleted model/sample: the middleware the
// copy wrote still carries the module's ownership marker, is still registered
// in middleware/middleware.go, and imports service/sample/session, which
// nothing else imports. clean sets --clean-orphans for the test.
func setupRemovedModuleProject(t *testing.T, clean bool) (middlewareFile, registrationFile, helperFile string) {
	t.Helper()

	oldModule, oldCleanOrphans := module, cleanOrphans
	t.Cleanup(func() {
		module, cleanOrphans = oldModule, oldCleanOrphans
	})
	module, cleanOrphans = "tmpapp", clean
	t.Chdir(t.TempDir())

	middlewareFile = filepath.Join(ggconst.DirMiddleware, "sample_auth.go")
	registrationFile = filepath.Join(ggconst.DirMiddleware, "middleware.go")
	helperFile = filepath.Join(ggconst.DirService, "sample", "session", "session.go")
	writeProjectFile(t, middlewareFile, `// Managed by gg module copy (module sample). Removing the module removes this file.

package middleware

import "tmpapp/service/sample/session"

func SampleAuth() any {
	return session.Check
}
`)
	writeProjectFile(t, registrationFile, `package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(SampleAuth())
}
`)
	writeProjectFile(t, helperFile, "package session\n\nvar Check any\n")
	return middlewareFile, registrationFile, helperFile
}

// withStdin runs fn with os.Stdin reading input.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		_ = reader.Close()
	})
	fn()
}
