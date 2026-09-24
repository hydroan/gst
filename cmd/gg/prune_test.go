package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// TestPruneLeftoversKeepsWhatPruneIgnoreCovers pins that prune never
// deletes what a gst.yaml prune.ignore entry covers: a disabled service file,
// a whole directory of them, or a directory left empty. An entry naming
// nothing on disk is warned about.
func TestPruneLeftoversKeepsWhatPruneIgnoreCovers(t *testing.T) {
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
			pruneLeftovers([]string{listFile, legacyFile}, nil, nil, nil, gghelper.NewProjectIgnore(), protect)
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

// TestPruneLeftoversRemindsOfUnreadSettingsBeforeAsking pins that with an
// old .gg.yaml next to gst.yaml, prune says right before asking to delete that
// the paths it lists are not protected.
func TestPruneLeftoversRemindsOfUnreadSettingsBeforeAsking(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectFile(t, ".gg.yaml", "prune:\n  ignore:\n    - service/record\n")
	listFile := filepath.Join(ggconst.DirService, "record", "list.go")
	writeProjectFile(t, listFile, "package record\n")

	var stdout string
	withStdin(t, "n\n", func() {
		stdout = captureStdout(t, func() {
			pruneLeftovers([]string{listFile}, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
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

// TestPruneLeftoversListsEverythingAndAsksOnce pins that prune works out all
// it deletes before it asks, and asks once: the service file of a disabled
// action, the directory only that file imports, an orphan because the file
// goes, and the middleware of a removed copied module with the directory only
// it imports are listed together ahead of the one question. Yes deletes them
// all, together with the directories this leaves empty, and keeps the service
// file a model still expects; any other answer deletes nothing.
func TestPruneLeftoversListsEverythingAndAsksOnce(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		deleted bool
	}{
		{name: "yes", answer: "y\n", deleted: true},
		{name: "any other answer", answer: "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middlewareFile, _, moduleHelperFile := setupRemovedModuleProject(t)
			currentFile := filepath.Join(ggconst.DirService, "authz", "role", "role.go")
			disabledFile := filepath.Join(ggconst.DirService, "authz", "role", "list.go")
			helperFile := filepath.Join(ggconst.DirService, "shared", "helper", "helper.go")
			writeProjectFile(t, currentFile, "package role\n")
			writeProjectFile(t, disabledFile, "package role\n\nimport _ \"tmpapp/service/shared/helper\"\n")
			writeProjectFile(t, helperFile, "package helper\n")

			var stdout string
			withStdin(t, tt.answer, func() {
				stdout = captureStdout(t, func() {
					pruneLeftovers([]string{currentFile, disabledFile}, []*gen.ModelInfo{pruneTestModel()}, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
				})
			})

			const question = "Do you want to delete these files?"
			if n := strings.Count(stdout, question); n != 1 {
				t.Fatalf("asked %d times, want once:\n%s", n, stdout)
			}
			leftovers := []string{disabledFile, helperFile, middlewareFile, moduleHelperFile}
			for _, path := range leftovers {
				if i := strings.Index(stdout, path); i < 0 || i > strings.Index(stdout, question) {
					t.Errorf("%s should be listed ahead of the question:\n%s", path, stdout)
				}
			}
			for _, path := range leftovers {
				if _, err := os.Stat(path); os.IsNotExist(err) != tt.deleted {
					t.Errorf("%s deleted = %t, want %t", path, os.IsNotExist(err), tt.deleted)
				}
			}
			if _, err := os.Stat(currentFile); err != nil {
				t.Errorf("%s is still expected and should stay: %v", currentFile, err)
			}
			if _, err := os.Stat(filepath.Join(ggconst.DirService, "shared")); os.IsNotExist(err) != tt.deleted {
				t.Errorf("service/shared removed = %t, want %t", os.IsNotExist(err), tt.deleted)
			}
		})
	}
}

// TestPruneLeftoversCleansUpAfterARemovedCopiedModule pins that the removal
// path gg module copy prints, deleting model/<name> and then pruning, takes
// the middleware the copy wrote as well: the file carrying the module's
// ownership marker, its register calls, and the service directory only that
// middleware imported. A prune.ignore entry keeps the file for good, together
// with what it imports. Only the orphan directory's files draw the warning
// about files gg cannot prove it owns. Any answer but yes keeps everything,
// and so does a middleware file prune cannot delete or a middleware directory
// it cannot read: the service directory is an orphan only because the
// middleware goes.
func TestPruneLeftoversCleansUpAfterARemovedCopiedModule(t *testing.T) {
	t.Run("cleaned on yes", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t)

		var stdout string
		withStdin(t, "y\n", func() {
			stdout = captureStdout(t, func() {
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
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
		for _, want := range []string{
			"Orphan Module Middleware Files",
			middlewareFile + " (copied with module sample, whose model/sample is gone",
			"This will delete unmanaged files that gg cannot prove it owns.",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("output lacks %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("kept by prune.ignore", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t)
		protect := ggconfig.PruneConfig{Ignore: []string{filepath.ToSlash(middlewareFile)}}

		withStdin(t, "y\n", func() {
			captureStdout(t, func() {
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), protect)
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

	t.Run("canceled", func(t *testing.T) {
		middlewareFile, registrationFile, _ := setupRemovedModuleProject(t)
		// The module's service directory is already gone, so the marked
		// middleware is all there is to clean.
		if err := os.RemoveAll(filepath.Join(ggconst.DirService, "sample")); err != nil {
			t.Fatal(err)
		}

		var stdout string
		withStdin(t, "no\n", func() {
			stdout = captureStdout(t, func() {
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
			})
		})

		if _, err := os.Stat(middlewareFile); err != nil {
			t.Errorf("%s should be kept when the deletion is canceled: %v", middlewareFile, err)
		}
		registration, err := os.ReadFile(registrationFile)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(registration), "middleware.RegisterAuth(SampleAuth())") {
			t.Errorf("%s lost a register call although the deletion was canceled:\n%s", registrationFile, registration)
		}
		if !strings.Contains(stdout, "Deletion canceled") || strings.Contains(stdout, "cannot prove it owns") {
			t.Errorf("want the cancel reported without the warning about unmanaged files, since only marked middleware is listed:\n%s", stdout)
		}
	})

	t.Run("kept when the middleware cannot be deleted", func(t *testing.T) {
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t)
		// Prune reads the functions a middleware file declares before deleting
		// it, to find their register calls; a file that stopped parsing ends
		// the cleanup right there.
		writeProjectFile(t, middlewareFile, `// Managed by gg module copy (module sample). Removing the module removes this file.

package middleware

import "tmpapp/service/sample/session"

func SampleAuth() any {
	return session.Check
`)

		var stdout string
		withStdin(t, "y\n", func() {
			stdout = captureStdout(t, func() {
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
			})
		})

		for _, path := range []string{middlewareFile, registrationFile, helperFile} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s should be kept when the middleware cannot be deleted: %v", path, err)
			}
		}
		if !strings.Contains(stdout, "Failed to delete orphan module middleware, so orphan service directories are kept") {
			t.Errorf("output lacks the failure and what it keeps:\n%s", stdout)
		}
	})

	t.Run("kept when the middleware directory cannot be read", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a directory whatever its permissions")
		}
		middlewareFile, registrationFile, helperFile := setupRemovedModuleProject(t)
		dir, err := filepath.Abs(ggconst.DirMiddleware)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(dir, 0o000); err != nil {
			t.Fatal(err)
		}
		// Give the permission back before the temporary directory is removed.
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		var stdout string
		withStdin(t, "y\n", func() {
			stdout = captureStdout(t, func() {
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{})
			})
		})

		if err = os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{middlewareFile, registrationFile, helperFile} {
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("%s should be kept when the middleware directory cannot be read: %v", path, statErr)
			}
		}
		if !strings.Contains(stdout, "failed to read the middleware directory, so orphans are not checked") {
			t.Errorf("output lacks the warning about the unreadable middleware directory:\n%s", stdout)
		}
	})
}

// setupRemovedModuleProject moves the test into a project, module tmpapp, that
// copied the module sample and then deleted model/sample: the middleware the
// copy wrote still carries the module's ownership marker, is still registered
// in middleware/middleware.go, and imports service/sample/session, which
// nothing else imports.
func setupRemovedModuleProject(t *testing.T) (middlewareFile, registrationFile, helperFile string) {
	t.Helper()

	oldModule := module
	t.Cleanup(func() {
		module = oldModule
	})
	module = "tmpapp"
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

// pruneTestModel returns a model of module tmpapp whose only enabled action, a
// Create, writes service/authz/role/role.go, so service/authz/role is a
// directory a model owns and every other phase file there, list.go among
// them, belongs to a disabled action.
func pruneTestModel() *gen.ModelInfo {
	disabled := func(phase consts.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join(ggconst.DirModel, "authz"),
		ModelFilePath: filepath.Join(ggconst.DirModel, "authz", "role.go"),
		Design: &dsl.Design{
			Enabled:    true,
			Endpoint:   "authz/roles",
			Create:     &dsl.Action{Enabled: true, Service: true, Filename: "role.go", Phase: consts.PHASE_CREATE},
			Delete:     disabled(consts.PHASE_DELETE),
			Update:     disabled(consts.PHASE_UPDATE),
			Patch:      disabled(consts.PHASE_PATCH),
			List:       disabled(consts.PHASE_LIST),
			Get:        disabled(consts.PHASE_GET),
			CreateMany: disabled(consts.PHASE_CREATE_MANY),
			DeleteMany: disabled(consts.PHASE_DELETE_MANY),
			UpdateMany: disabled(consts.PHASE_UPDATE_MANY),
			PatchMany:  disabled(consts.PHASE_PATCH_MANY),
			Import:     disabled(consts.PHASE_IMPORT),
			Export:     disabled(consts.PHASE_EXPORT),
			SSE:        disabled(consts.PHASE_SSE),
		},
	}
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
