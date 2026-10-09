// Package modelinspect inspects a project's models by running them. It builds
// a program into the project's module that loads the model packages, has gorm
// parse every model and reports what only the compiled models can tell: the
// database columns behind the fields, for the column references gg gen writes,
// and what the index declarations violate on MySQL, for gg check. Both lines
// of gg depend on this package and on nothing of each other's. An inspection
// is cached by its inputs under the user cache directory, so a project one
// command inspected is not built again by the other.
package modelinspect

import (
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// ColumnInfo is one column of a model, as the inspection program reports it.
type ColumnInfo struct {
	GoName      string            `json:"go_name"`
	DBName      string            `json:"db_name"`
	TypeExpr    string            `json:"type_expr"`    // Source-level type expression, empty when the type cannot be reproduced.
	TypeImports map[string]string `json:"type_imports"` // The imports TypeExpr needs, by path, each under the package name it qualifies the type with; none for builtin or same-package types.
	TypeName    string            `json:"type_name"`    // Original type, recorded in a comment when TypeExpr is empty.
	Numeric     bool              `json:"numeric"`      // Column type is a numeric kind, so the reference gains SUM and AVG.
	Time        bool              `json:"time"`         // Column type is time.Time, so the reference gains time bucketing.
}

// ModelColumns is one model as the inspection program reports it: its columns,
// and what model.ValidateIndexes refused in its index declarations on MySQL,
// "" when nothing.
type ModelColumns struct {
	PkgPath        string       `json:"pkg_path"`
	PkgName        string       `json:"pkg_name"`
	Name           string       `json:"name"`
	Columns        []ColumnInfo `json:"columns"`
	IndexViolation string       `json:"index_violation"`
}

// Inspect inspects the project's models by running them: the inspection
// program (see buildProgram) is built into the project's module and reports
// every model's columns and what model.ValidateIndexes refused in its index
// declarations on MySQL. The result comes from the cache when every input of
// the inspection is unchanged, otherwise from a build and run of the program.
// The build takes the files overlay returns in place of the project's own,
// which gg gen uses to stub out the column files it is about to rewrite; a
// nil overlay builds the project as it is. A project that does not depend on
// the framework yet cannot be inspected and reports inspected false: the
// program imports the project's models against the framework, and such a
// project cannot hold column references either.
func Inspect(module string, modelDir string, models []*modelinfo.Model, ignore gghelper.ProjectIgnore, overlay func() (map[string]string, error)) ([]ModelColumns, bool, error) {
	dependsOnGst, err := gghelper.RequiresFramework(".")
	if err != nil || !dependsOnGst {
		return nil, false, err
	}

	program := buildProgram(module, models)

	// Compiling and running the inspection program costs seconds, which would
	// otherwise be paid on every run even when nothing that affects the
	// result changed. The cache key covers every such input, so a hit skips
	// the build entirely and a miss is unavoidable work.
	cacheKey, err := inspectionCacheKey(program, modelDir, ignore)
	if err != nil {
		return nil, false, err
	}
	resolved, cached := readInspectionCache(cacheKey)
	if !cached {
		var files map[string]string
		if overlay != nil {
			if files, err = overlay(); err != nil {
				return nil, false, err
			}
		}
		if resolved, err = runProgram(program, files); err != nil {
			return nil, false, err
		}
		if err = writeInspectionCache(cacheKey, resolved); err != nil {
			return nil, false, err
		}
	}
	return resolved, true, nil
}
