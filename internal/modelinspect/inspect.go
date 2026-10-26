// Package modelinspect inspects a project's models by running them. It builds
// a program into the project's module that loads the model packages, has gorm
// parse every model and reports what only the compiled models can tell: the
// database columns behind the fields, for the column references gg gen
// writes, and for gg check what the index declarations violate on MySQL,
// which framework columns a model renames and which query names it declares
// ambiguously. Both lines of gg depend on this package and on nothing of each
// other's. An inspection is cached by its inputs under the user cache
// directory, so a project one command inspected is not built again by the
// other.
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
	Numeric     bool              `json:"numeric"`      // modelschema.ClassifyColumn calls the column numeric, so the reference gains SUM and AVG.
	Time        bool              `json:"time"`         // modelschema.ClassifyColumn calls the column a time, so the reference gains time bucketing.
	JSONStrings bool              `json:"json_strings"` // Column type stores a JSON array of strings, which gets the JSONStringsColumn reference.
}

// ModelColumns is one model as the inspection program reports it: its columns,
// what model.ValidateIndexes refused in its index declarations on MySQL, what
// modelschema.ValidateFrameworkColumns refused in its columns and what
// urlquery.ValidateQueryNames refused in its query tags, each "" when nothing.
type ModelColumns struct {
	PkgPath                  string       `json:"pkg_path"`
	PkgName                  string       `json:"pkg_name"`
	Name                     string       `json:"name"`
	Columns                  []ColumnInfo `json:"columns"`
	IndexViolation           string       `json:"index_violation"`
	FrameworkColumnViolation string       `json:"framework_column_violation"`
	QueryNameViolation       string       `json:"query_name_violation"`
}

// Inspect inspects the project's models by running them: the inspection
// program (see buildProgram) is built into the project's module and reports
// every model's columns, what model.ValidateIndexes refused in its index
// declarations on MySQL, what modelschema.ValidateFrameworkColumns refused in
// its columns and what urlquery.ValidateQueryNames refused in its query
// tags. The result comes from the cache when every input of the inspection
// is unchanged, otherwise from a build and run of the program. The build
// takes the files overlay returns in place of the project's own, which gg gen
// uses to stub out the column files it is about to rewrite; a nil overlay
// builds the project as it is. A project that does not depend on the
// framework yet cannot be inspected and reports inspected false: the program
// imports the project's models against the framework, and such a project
// cannot hold column references either.
func Inspect(module string, modelDir string, models []*modelinfo.Model, ignore gghelper.ProjectIgnore, overlay func() (map[string]string, error)) ([]ModelColumns, bool, error) {
	dependsOnGst, err := gghelper.RequiresFramework(".")
	if err != nil || !dependsOnGst {
		return nil, false, err
	}

	files := gghelper.InspectProgram()
	if files[projectFileName], err = buildProjectFile(module, models); err != nil {
		return nil, false, err
	}

	// Compiling and running the inspection program costs seconds, which would
	// otherwise be paid on every run even when nothing that affects the
	// result changed. The cache key covers every such input, so a hit skips
	// the build entirely and a miss is unavoidable work.
	cacheKey, err := inspectionCacheKey(files, modelDir, ignore)
	if err != nil {
		return nil, false, err
	}
	resolved, cached := readInspectionCache(cacheKey)
	if !cached {
		var replacements map[string]string
		if overlay != nil {
			if replacements, err = overlay(); err != nil {
				return nil, false, err
			}
		}
		if resolved, err = runProgram(files, replacements); err != nil {
			return nil, false, err
		}
		if err = writeInspectionCache(cacheKey, resolved); err != nil {
			return nil, false, err
		}
	}
	return resolved, true, nil
}
