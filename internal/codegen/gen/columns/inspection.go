package columns

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/gghelper"
)

// columnInfo is one generated column reference, as reported by the inspection
// program that runs inside the project module.
type columnInfo struct {
	GoName   string `json:"go_name"`
	DBName   string `json:"db_name"`
	TypeExpr string `json:"type_expr"` // Source-level type expression, empty when the type cannot be reproduced.
	TypePkg  string `json:"type_pkg"`  // Import path required by TypeExpr, empty for builtin or same-package types.
	TypeName string `json:"type_name"` // Original type, recorded in a comment when TypeExpr is empty.
	Numeric  bool   `json:"numeric"`   // Column type is a numeric kind, so the reference gains SUM and AVG.
	Time     bool   `json:"time"`      // Column type is time.Time, so the reference gains time bucketing.
}

// modelColumns groups the columns of one model.
type modelColumns struct {
	PkgPath string       `json:"pkg_path"`
	PkgName string       `json:"pkg_name"`
	Name    string       `json:"name"`
	Columns []columnInfo `json:"columns"`
}

// columnsProgram is the template of the inspection program that reports the
// project models' columns as JSON. It runs inside the project module, so it
// resolves exactly the columns the framework resolves at runtime.
// buildColumnsProgram fills {{MODULE}} and the unregistered-model
// placeholders; {{OUTPUT}} is filled per run.
const columnsProgram = `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/modelschema"

	_ "{{MODULE}}/model"
{{UNREGISTERED_IMPORTS}})

type columnInfo struct {
	GoName   string ` + "`json:\"go_name\"`" + `
	DBName   string ` + "`json:\"db_name\"`" + `
	TypeExpr string ` + "`json:\"type_expr\"`" + `
	TypePkg  string ` + "`json:\"type_pkg\"`" + `
	TypeName string ` + "`json:\"type_name\"`" + `
	Numeric  bool   ` + "`json:\"numeric\"`" + `
	Time     bool   ` + "`json:\"time\"`" + `
}

type modelColumns struct {
	PkgPath string       ` + "`json:\"pkg_path\"`" + `
	PkgName string       ` + "`json:\"pkg_name\"`" + `
	Name    string       ` + "`json:\"name\"`" + `
	Columns []columnInfo ` + "`json:\"columns\"`" + `
}

func main() {
	// Only the configuration is loaded, the way the service loads it: gorm's
	// parser calls methods on the models and their field types, and project
	// code in them may read it. The models come from the model packages
	// imported above; resolving their columns never touches the database.
	if err := config.Init(); err != nil {
		fail(err)
	}
	defer config.Clean()

	seen := make(map[string]struct{})
	out := make([]modelColumns, 0)
	models := model.RegisteredModels()
{{UNREGISTERED_MODELS}}	for _, m := range models {
		typ := reflect.TypeOf(m)
		for typ != nil && typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ == nil || typ.PkgPath() == "" || typ.Name() == "" {
			continue
		}
		key := typ.PkgPath() + "." + typ.Name()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		cols, err := modelschema.Columns(typ)
		if err != nil {
			fail(fmt.Errorf("resolve columns of %s: %w", key, err))
		}
		entry := modelColumns{PkgPath: typ.PkgPath(), PkgName: packageName(typ), Name: typ.Name()}
		for _, col := range cols {
			expr, pkg := describeType(col.Type, typ.PkgPath())
			class := modelschema.ClassifyColumn(col.Type)
			entry.Columns = append(entry.Columns, columnInfo{
				GoName:   col.GoName,
				DBName:   col.DBName,
				TypeExpr: expr,
				TypePkg:  pkg,
				TypeName: col.Type.String(),
				Numeric:  class == modelschema.ColumnClassNumeric,
				Time:     class == modelschema.ColumnClassTime,
			})
		}
		out = append(out, entry)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		fail(err)
	}
	// The result travels through a file: initialization writes to stdout, so
	// stdout is not a reliable data channel.
	if err = os.WriteFile("{{OUTPUT}}", encoded, 0o600); err != nil {
		fail(err)
	}
}

// packageName returns the package name the compiler recorded for the type,
// which can differ from the last path segment.
func packageName(typ reflect.Type) string {
	full := typ.String()
	if idx := strings.LastIndex(full, "."); idx >= 0 {
		return full[:idx]
	}
	return ""
}

// describeType renders a column type as a source-level expression plus the
// import it needs. Pointers are dereferenced, since a filter compares the
// pointed-to value. A type whose name cannot be written back as source, such
// as a generic instantiation, yields an empty expression and is generated as
// Column[any]: the column name stays exact and the JSON operators, which take
// a string, keep working.
func describeType(typ reflect.Type, modelPkg string) (expr string, importPath string) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() == "" {
		// Builtin or composite of builtins: string, int64, []uint8.
		return typ.String(), ""
	}
	name := typ.Name()
	if name == "" || strings.ContainsAny(name, "[]*") {
		return "", ""
	}
	if typ.PkgPath() == modelPkg {
		return name, ""
	}
	// Framework-internal types reach business models only through the public
	// packages that alias them. Reflection sees the defined type's internal
	// path, which a business project cannot import, so the reference is
	// rewritten to the alias the model source actually wrote: model.Version,
	// model.Base and their siblings for internal/modelregistry, and the root gst
	// package, which forwards everything internal/types defines under the same
	// name.
	switch typ.PkgPath() {
	case "github.com/hydroan/gst/internal/modelregistry":
		return "model." + name, "github.com/hydroan/gst/model"
	case "github.com/hydroan/gst/internal/types":
		return "gst." + name, "github.com/hydroan/gst"
	}
	// typ.String() carries the package name the compiler recorded, which the
	// generated file then imports under that exact alias.
	return typ.String(), typ.PkgPath()
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
`

// buildColumnsProgram renders the inspection program for the project.
// Registered models are enumerated at run time through
// model.RegisteredModels; a model that declares a Design but no Migrate never
// reaches the registry, so it is compiled into the program as an explicit
// entry instead and kept at run time only when it opted in to framework query
// parameters — that opt-in is what gives it a filter and sort column
// namespace worth generating references for.
//
// Models ignored by gst.yaml gen.models.ignore are also compiled in as
// explicit entries but skip the query-parameter gate: they remain
// table-backed and their column files must not drift from the sources.
func buildColumnsProgram(module string, models []*gen.ModelInfo) string {
	program := strings.ReplaceAll(columnsProgram, "{{MODULE}}", module)

	unregistered := make([]*gen.ModelInfo, 0, len(models))
	ignored := make([]*gen.ModelInfo, 0, len(models))
	for _, m := range models {
		switch {
		case m.RegisterIgnored:
			ignored = append(ignored, m)
		case m.Design.Enabled && !m.Design.Migrate:
			unregistered = append(unregistered, m)
		}
	}

	// One deterministic alias per package: the fixed prefix cannot collide
	// with the template's own imports, and sorting keeps the program text,
	// and with it the inspection cache key, stable across runs.
	extra := make([]*gen.ModelInfo, 0, len(unregistered)+len(ignored))
	extra = append(extra, unregistered...)
	extra = append(extra, ignored...)
	aliases := make(map[string]string, len(extra))
	paths := make([]string, 0, len(extra))
	for _, m := range extra {
		path := modelPkgPath(m)
		if _, ok := aliases[path]; !ok {
			aliases[path] = ""
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for i, path := range paths {
		aliases[path] = fmt.Sprintf("vm%d", i)
	}
	sortByPackageAndName := func(entries []*gen.ModelInfo) {
		sort.Slice(entries, func(i, j int) bool {
			if pi, pj := modelPkgPath(entries[i]), modelPkgPath(entries[j]); pi != pj {
				return pi < pj
			}
			return entries[i].ModelName < entries[j].ModelName
		})
	}
	sortByPackageAndName(unregistered)
	sortByPackageAndName(ignored)

	var imports strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&imports, "\t%s %q\n", aliases[path], path)
	}

	var entries strings.Builder
	if len(unregistered) > 0 {
		entries.WriteString(`	// Models that declare a Design but no Migrate never reach the registry.
	// Their query columns resolve the same way, so those that opted in to
	// framework query parameters are inspected alongside the registered ones.
	for _, m := range []any{
`)
		for _, m := range unregistered {
			fmt.Fprintf(&entries, "\t\t&%s.%s{},\n", aliases[modelPkgPath(m)], m.ModelName)
		}
		entries.WriteString(`	} {
		if !modelschema.IsQueryable(m) {
			continue
		}
		models = append(models, m)
	}
`)
	}
	if len(ignored) > 0 {
		entries.WriteString(`	// Models whose registration is ignored by gst.yaml gen.models.ignore
	// stay table-backed: their column files must keep matching the
	// module-copied model sources, so they are inspected unconditionally.
	models = append(models,
`)
		for _, m := range ignored {
			fmt.Fprintf(&entries, "\t\t&%s.%s{},\n", aliases[modelPkgPath(m)], m.ModelName)
		}
		entries.WriteString("\t)\n")
	}

	program = strings.ReplaceAll(program, "{{UNREGISTERED_IMPORTS}}", imports.String())
	return strings.ReplaceAll(program, "{{UNREGISTERED_MODELS}}", entries.String())
}

// inspectColumns compiles and runs the inspection program and decodes what it
// reports. The result travels through a file rather than stdout, because
// framework initialization writes progress lines to stdout. The build runs
// with the overlay columnInspectionOverlay returns, so neither a stale
// generation nor the handwritten code reading it blocks the run that would
// refresh it.
func inspectColumns(program string, overlay map[string]string) ([]modelColumns, error) {
	resultFile, err := os.CreateTemp("", "gg-columns-*.json")
	if err != nil {
		return nil, errors.Wrap(err, "create column result file")
	}
	resultPath := resultFile.Name()
	if err = resultFile.Close(); err != nil {
		return nil, errors.Wrap(err, "close column result file")
	}
	defer os.Remove(resultPath)

	inspector := gghelper.ProjectProgram{Content: strings.ReplaceAll(program, "{{OUTPUT}}", resultPath), Overlay: overlay}
	if err = inspector.Run(); err != nil {
		return nil, errors.Wrap(err, "inspect model columns")
	}
	output, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, errors.Wrap(err, "read resolved columns")
	}
	var resolved []modelColumns
	if err = json.Unmarshal(output, &resolved); err != nil {
		return nil, errors.Wrap(err, "decode model columns")
	}
	return resolved, nil
}
