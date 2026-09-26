package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// TestPruneLeftoversDeletesStalePBFiles pins that the files under pb/ gg gen
// would not write now, definitions, the Go files serving them and the Go
// files compiled from them, are listed under their own heading ahead of the
// question, deleted on yes with the directories that leaves empty, and kept
// when a gst.yaml prune.ignore entry covers them, listed among the files
// ignored by config.
func TestPruneLeftoversDeletesStalePBFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	current := filepath.Join(ggconst.DirPB, "record.proto")
	currentGo := filepath.Join(ggconst.DirPB, "record.pb.go")
	currentHandlers := filepath.Join(ggconst.DirPB, "record.gen.go")
	registration := filepath.Join(ggconst.DirPB, ggconst.FilePBGen)
	stale := filepath.Join(ggconst.DirPB, "archive", "note.proto")
	staleGo := filepath.Join(ggconst.DirPB, "archive", "note_grpc.pb.go")
	staleHandlers := filepath.Join(ggconst.DirPB, "archive", "note.gen.go")
	kept := filepath.Join(ggconst.DirPB, "legacy", "item.proto")
	for _, path := range []string{current, stale, kept} {
		writeProjectFile(t, path, "syntax = \"proto3\";\n")
	}
	// The Go files have to parse: prune reads every Go file of the project
	// to trace which service directories live code imports.
	for _, path := range []string{currentGo, currentHandlers, registration} {
		writeProjectFile(t, path, "package pb\n")
	}
	for _, path := range []string{staleGo, staleHandlers} {
		writeProjectFile(t, path, "package archive\n")
	}
	protect := ggconfig.PruneConfig{Ignore: []string{"pb/legacy"}}

	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), protect,
				[]string{current, currentGo, currentHandlers, registration, stale, staleGo, staleHandlers, kept},
				[]string{"pb/record.proto", "pb/record.gen.go", "pb/pb.gen.go", "pb/record.pb.go"})
		})
	})

	const question = "Do you want to delete these files?"
	heading := strings.Index(stdout, "Stale Protobuf Files")
	if heading < 0 || heading > strings.Index(stdout, question) {
		t.Fatalf("the stale files should be listed ahead of the question:\n%s", stdout)
	}
	for _, path := range []string{stale, staleGo, staleHandlers} {
		if i := strings.Index(stdout, path); i < heading || i > strings.Index(stdout, question) {
			t.Errorf("%s should be listed under the heading:\n%s", path, stdout)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be deleted; stat error = %v", path, err)
		}
	}
	if !strings.Contains(stdout[:heading], "ignore "+kept) {
		t.Errorf("%s should be listed among the files ignored by config:\n%s", kept, stdout)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("%s is left empty and should be removed; stat error = %v", filepath.Dir(stale), err)
	}
	for _, path := range []string{current, currentGo, currentHandlers, registration, kept} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should survive prune: %v", path, err)
		}
	}
}

// TestCompiledPBPathsNamesWhatCompileWrites holds the names prune derives
// for the Go files to the ones the protobuf plugins actually write: a
// definition with a service gets both files, one without gets the messages
// file alone, and a Go file gg gen wrote itself gets none.
func TestCompiledPBPathsNamesWhatCompileWrites(t *testing.T) {
	protos := []pb.File{
		{Path: "pb/sample.proto", Service: true, Content: `syntax = "proto3";

package tmpapp;

option go_package = "tmpapp/pb;pb";

message Sample {
  string id = 1;
}

message GetSampleRequest {
  string id = 1;
}

message GetSampleResponse {
  Sample sample = 1;
}

service SampleService {
  rpc GetSample ( GetSampleRequest ) returns ( GetSampleResponse );
}
`},
		{Path: "pb/record/types.proto", Content: `syntax = "proto3";

package tmpapp.record;

option go_package = "tmpapp/pb/record;record";

message Link {
  string url = 1;
}
`},
	}

	compiled, err := pb.Compile(protos)
	if err != nil {
		t.Fatal(err)
	}
	written := make([]string, 0, len(compiled))
	for _, f := range compiled {
		written = append(written, f.Path)
	}
	derived := compiledPBPaths(append(protos, pb.File{Path: "pb/sample.gen.go", Content: "package pb\n"}, pb.File{Path: "pb/pb.gen.go", Content: "package pb\n"}))
	slices.Sort(derived)
	if !slices.Equal(derived, written) {
		t.Fatalf("compiledPBPaths() = %v, pb.Compile wrote %v", derived, written)
	}
	if want := []string{"pb/record/types.pb.go", "pb/sample.pb.go", "pb/sample_grpc.pb.go"}; !slices.Equal(written, want) {
		t.Fatalf("pb.Compile wrote %v, want %v", written, want)
	}
}

// TestPruneRunDeletesTheFilesOfAModelNoLongerServedOverGRPC pins the whole
// path: gg gen writes pb/note.proto for a model declaring GRPC(), with
// pb/note.gen.go serving it, pb/pb.gen.go registering it and pb/note.pb.go
// and pb/note_grpc.pb.go compiled from it, and once the declaration is gone
// gg prune lists and deletes the five, with the pb directory it leaves
// empty.
func TestPruneRunDeletesTheFilesOfAModelNoLongerServedOverGRPC(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	if err := genRunWithOptions(genRunOptions{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	generated := []string{
		filepath.Join(ggconst.DirPB, "note.proto"),
		filepath.Join(ggconst.DirPB, "note.gen.go"),
		filepath.Join(ggconst.DirPB, ggconst.FilePBGen),
		filepath.Join(ggconst.DirPB, "note.pb.go"),
		filepath.Join(ggconst.DirPB, "note_grpc.pb.go"),
	}
	for _, path := range generated {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("gg gen should have written %s: %v", path, err)
		}
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "\tdsl.GRPC()\n", "", 1)})

	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			if err := pruneRun(); err != nil {
				t.Error(err)
			}
		})
	})

	for _, path := range generated {
		if !strings.Contains(stdout, "Stale Protobuf Files") || !strings.Contains(stdout, path) {
			t.Errorf("prune should list %s under Stale Protobuf Files:\n%s", path, stdout)
		}
	}
	if _, err := os.Stat(ggconst.DirPB); !os.IsNotExist(err) {
		t.Errorf("%s should be gone with its last file; stat error = %v", ggconst.DirPB, err)
	}
}

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
			pruneLeftovers([]string{listFile, legacyFile}, nil, nil, nil, gghelper.NewProjectIgnore(), protect, nil, nil)
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
			pruneLeftovers([]string{listFile}, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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
// service/, middleware/ and pb/, as an error the command prints, not as a
// panic.
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
					pruneLeftovers([]string{currentFile, disabledFile}, []*modelinfo.Model{pruneTestModel()}, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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

// TestPruneLeftoversDeletesThePairedTestFiles pins that a disabled service
// file takes its test file with it, listed ahead of the question, while the
// test files of the package's other service file and its main_test.go stay.
func TestPruneLeftoversDeletesThePairedTestFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectGoMod(t, ".")
	dir := filepath.Join(ggconst.DirService, "authz", "role")
	currentFile := filepath.Join(dir, "role.go")
	currentTest := filepath.Join(dir, "role_test.go")
	mainTest := filepath.Join(dir, "main_test.go")
	disabledFile := filepath.Join(dir, "list.go")
	disabledTest := filepath.Join(dir, "list_test.go")
	for _, path := range []string{currentFile, disabledFile} {
		writeProjectFile(t, path, "package role\n")
	}
	for _, path := range []string{currentTest, mainTest, disabledTest} {
		writeProjectFile(t, path, "package role_test\n")
	}

	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			pruneLeftovers([]string{currentFile, disabledFile}, []*modelinfo.Model{pruneTestModel()}, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
		})
	})

	const question = "Do you want to delete these files?"
	if i := strings.Index(stdout, disabledTest); i < 0 || i > strings.Index(stdout, question) {
		t.Errorf("%s should be listed ahead of the question:\n%s", disabledTest, stdout)
	}
	for _, path := range []string{disabledFile, disabledTest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be deleted, stat error = %v", path, err)
		}
	}
	for _, path := range []string{currentFile, currentTest, mainTest} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should stay: %v", path, err)
		}
	}
}

// TestPruneLeftoversCleansTheWholePackage pins what happens to the test
// files of a package whose last service file goes: the paired test file is
// listed once, with its service file, and the test files pairing with none,
// main_test.go and fixtures_test.go, go with the orphan directory, so
// nothing is deleted twice and the directory is removed.
func TestPruneLeftoversCleansTheWholePackage(t *testing.T) {
	t.Chdir(t.TempDir())
	writeProjectGoMod(t, ".")
	dir := filepath.Join(ggconst.DirService, "authz", "role")
	disabledFile := filepath.Join(dir, "list.go")
	disabledTest := filepath.Join(dir, "list_test.go")
	mainTest := filepath.Join(dir, "main_test.go")
	fixturesTest := filepath.Join(dir, "fixtures_test.go")
	writeProjectFile(t, disabledFile, "package role\n")
	for _, path := range []string{disabledTest, mainTest, fixturesTest} {
		writeProjectFile(t, path, "package role_test\n")
	}

	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			pruneLeftovers([]string{disabledFile}, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
		})
	})

	if n := strings.Count(stdout, disabledTest); n != 2 {
		t.Errorf("%s listed and deleted once each, want 2 mentions, got %d:\n%s", disabledTest, n, stdout)
	}
	if strings.Contains(stdout, "Failed to delete") {
		t.Errorf("nothing should fail to delete:\n%s", stdout)
	}
	for _, path := range []string{disabledFile, disabledTest, mainTest, fixturesTest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should be deleted, stat error = %v", path, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s should be removed once empty, stat error = %v", dir, err)
	}
}

// TestPruneLeftoversKeepsOrphansWhenADisabledFileStays pins the guard behind
// deleting everything in one run: a service directory can be an orphan only
// because the disabled service file importing it goes, so when a disabled file
// cannot be deleted, no orphan directory is, and the output says why. The
// middleware of a removed copied module still goes, while the directory only
// it imported stays with the other orphans.
func TestPruneLeftoversKeepsOrphansWhenADisabledFileStays(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes a file whatever the permissions of its directory")
	}
	middlewareFile, _, moduleHelperFile := setupRemovedModuleProject(t)
	roleDir := filepath.Join(ggconst.DirService, "authz", "role")
	currentFile := filepath.Join(roleDir, "role.go")
	disabledFile := filepath.Join(roleDir, "list.go")
	helperFile := filepath.Join(ggconst.DirService, "shared", "helper", "helper.go")
	writeProjectFile(t, currentFile, "package role\n")
	writeProjectFile(t, disabledFile, "package role\n\nimport _ \"tmpapp/service/shared/helper\"\n")
	writeProjectFile(t, helperFile, "package helper\n")
	// A file cannot be deleted from a directory the user may not write to.
	dir, err := filepath.Abs(roleDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	// Give the permission back before the temporary directory is removed.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	var stdout string
	withStdin(t, "y\n", func() {
		stdout = captureStdout(t, func() {
			pruneLeftovers([]string{currentFile, disabledFile}, []*modelinfo.Model{pruneTestModel()}, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
		})
	})

	for _, path := range []string{disabledFile, helperFile, moduleHelperFile} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should be kept: %v", path, err)
		}
	}
	if _, err := os.Stat(middlewareFile); !os.IsNotExist(err) {
		t.Errorf("%s does not depend on the disabled file and should be deleted, stat error = %v", middlewareFile, err)
	}
	for _, want := range []string{
		"Failed to delete " + disabledFile,
		"Some disabled service files were not deleted, so orphan service directories are kept",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
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
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), protect, nil, nil)
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
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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
				pruneLeftovers(nil, nil, nil, nil, gghelper.NewProjectIgnore(), ggconfig.PruneConfig{}, nil, nil)
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
		if !strings.Contains(stdout, "failed to read the middleware or interceptor directory, so orphans are not checked") {
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
func pruneTestModel() *modelinfo.Model {
	disabled := func(phase consts.Phase) *dsl.Action {
		return &dsl.Action{Phase: phase}
	}
	return &modelinfo.Model{
		ModulePath:    "tmpapp",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join(ggconst.DirModel, "authz"),
		ModelFilePath: filepath.Join(ggconst.DirModel, "authz", "role.go"),
		Design: &dsl.Design{
			Enabled:    true,
			Endpoint:   "authz/roles",
			Create:     &dsl.Action{Enabled: true, Service: true, Filename: "role.go", Phase: consts.Create},
			Delete:     disabled(consts.Delete),
			Update:     disabled(consts.Update),
			Patch:      disabled(consts.Patch),
			List:       disabled(consts.List),
			Get:        disabled(consts.Get),
			CreateMany: disabled(consts.CreateMany),
			DeleteMany: disabled(consts.DeleteMany),
			UpdateMany: disabled(consts.UpdateMany),
			PatchMany:  disabled(consts.PatchMany),
			Import:     disabled(consts.Import),
			Export:     disabled(consts.Export),
			SSE:        disabled(consts.SSE),
			Stream:     disabled(consts.Stream),
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
