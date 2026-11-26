package ggcheck

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/gggen/columns"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
	"github.com/hydroan/gst/internal/pbschema"
)

// GeneratedFiles holds the files gg gen writes anew on every run to what the
// models declare now.
var GeneratedFiles = Check{
	Name: "Generated files",
	Rule: "the files gg gen writes anew on every run — main.go, the registration files, the column reference files and the files under pb/ — must be the ones it writes for the models as they are: run gg gen after changing a model",
	run:  checkGeneratedFiles,
}

// checkGeneratedFiles renders the files gg gen writes anew on every run the
// way gg gen renders them, writing nothing, and reports each one the run
// would create or rewrite: main.go (see gggen.BuildMainFile), the
// registration files (see gggen.RegistrationFiles), the column reference
// files (see columns.Files) and the files under pb/ (see pb.Files). A model
// changed without a run after it leaves a project that builds and serves
// what the files held before: a route missing, removed or still public, a
// column reference or a protobuf field missing. The models are read the way
// gg gen reads them, the gst.yaml route and model ignores applied.
//
// The column reference files and the files under pb/ are rendered from the
// compiled models, so model packages that do not compile leave them
// unchecked, which is reported rather than passed, while the other files are
// still compared. What the protobuf derivation refuses is the Protobuf
// definitions check's to report and adds nothing here.
//
// The files gg gen writes once and leaves to the project, the service files
// and their tests, are not compared, nor are the TypeScript files of gg gen
// ts. A file gg gen would remove — the column references of a source no
// longer declaring a model, the Go files under pb/ of a model no longer
// served over gRPC — is not reported.
func checkGeneratedFiles(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	scanned, err := scanModels(ignore)
	if err != nil {
		return nil
	}

	// expected maps each file to the content gg gen writes it with; unchecked
	// names the groups that could not be rendered.
	expected := make(map[string]string)
	var unchecked []string

	registration, err := gggen.RegistrationFiles(scanned.Module, scanned.Models, ignore)
	if err != nil {
		unchecked = append(unchecked, fmt.Sprintf("the registration files could not be checked: %v", err))
	}
	maps.Copy(expected, registration)

	mainCode, err := gggen.BuildMainFile(scanned.Module, gghelper.PresentOptionalImportDirs()...)
	if err != nil {
		unchecked = append(unchecked, fmt.Sprintf("%s could not be checked: %v", ggconst.FileMain, err))
	} else {
		expected[ggconst.FileMain] = mainCode
	}

	resolved, inspected, err := modelinspect.Inspect(scanned.Module, ggconst.DirModel, scanned.Models, ignore, nil)
	if err == nil && inspected {
		var columnFiles map[string]string
		columnFiles, err = columns.Files(scanned.Module, ggconst.DirModel, scanned.Models, resolved)
		maps.Copy(expected, columnFiles)
	}
	if err != nil {
		unchecked = append(unchecked, fmt.Sprintf("the column reference files could not be checked: %v", err))
	}

	rendered, err := pb.Generate(pbschema.Config{Dir: ".", ModulePath: scanned.Module, Models: scanned.Models})
	var pbFiles []pb.File
	if err == nil {
		pbFiles, err = pb.Files(scanned.Module, rendered)
	}
	var diagnostics *pbschema.DiagnosticsError
	if err != nil && !errors.As(err, &diagnostics) {
		unchecked = append(unchecked, fmt.Sprintf("the files under %s/ could not be checked: %v", ggconst.DirPB, err))
	}
	for _, f := range pbFiles {
		expected[filepath.FromSlash(f.Path)] = f.Content
	}

	var violations []string
	for _, path := range slices.Sorted(maps.Keys(expected)) {
		current, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			violations = append(violations, path+": gg gen would create it; run gg gen")
		case err != nil:
			violations = append(violations, fmt.Sprintf("%s could not be read: %v", path, err))
		case string(current) != expected[path]:
			violations = append(violations, path+": gg gen would rewrite it; run gg gen")
		}
	}
	return append(violations, unchecked...)
}
