package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
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
	"github.com/hydroan/gst/module"

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
	// Only the pieces model registration depends on are initialized: resolving
	// columns never touches the database.
	if err := config.Init(); err != nil {
		fail(err)
	}
	defer config.Clean()
	if err := module.Init(); err != nil {
		fail(err)
	}

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
//
// When every scanned model is registered, the placeholders collapse to
// nothing and the program never references modelschema.IsQueryable, so such
// projects keep building against framework versions that do not export it.
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
	if len(unregistered) == 0 && len(ignored) == 0 {
		program = strings.ReplaceAll(program, "{{UNREGISTERED_IMPORTS}}", "")
		return strings.ReplaceAll(program, "{{UNREGISTERED_MODELS}}", "")
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

// generateColumnFiles writes one .gen.go file per model source file and
// removes the generated files whose source is gone. Generated files are
// framework-owned for their whole life cycle: projects never create, edit, or
// clean them up.
func generateColumnFiles(module string, modelDir string, models []*gen.ModelInfo, quiet bool) error {
	// Resolving columns compiles a program that imports the project's models,
	// which only works once the project depends on the framework. A project
	// that does not cannot hold column references either, so there is nothing
	// to generate yet.
	dependsOnGst, err := moduleRequiresGst()
	if err != nil {
		return err
	}
	if !dependsOnGst {
		return nil
	}

	program := buildColumnsProgram(module, models)

	// Compiling and running the inspection program costs seconds, which would
	// otherwise be paid on every gg gen even when nothing that affects columns
	// changed. The cache key covers every such input, so a hit skips the build
	// entirely and a miss is unavoidable work.
	cacheKey, err := columnsCacheKey(program, modelDir)
	if err != nil {
		return err
	}
	resolved, cached := readColumnsCache(cacheKey)
	if !cached {
		// The inspection build blanks out the previously generated column
		// files: they may carry an API shape older than the running gg, and
		// the build must not choke on the very files this run rewrites.
		stubs, stubErr := generatedColumnFileStubs(modelDir)
		if stubErr != nil {
			return stubErr
		}
		if resolved, err = inspectColumns(program, stubs); err != nil {
			return err
		}
		if err = writeColumnsCache(cacheKey, resolved); err != nil {
			return err
		}
	}

	// The scan that drives generation already knows which file declares each
	// model, so the resolved columns only need to be matched to it.
	sources := make(map[string]string, len(models))
	for _, m := range models {
		sources[modelPkgPath(m)+"."+m.ModelName] = m.ModelFilePath
	}

	byFile := groupColumnsByFile(resolved, sources)

	wanted := make(map[string]struct{}, len(byFile))
	for file, entries := range byFile {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		target := columnsFileName(file)
		wanted[target] = struct{}{}
		content, renderErr := renderColumnsFile(module, entries[0].PkgName, file, entries)
		if renderErr != nil {
			return renderErr
		}
		if writeErr := writeGeneratedFileIfChanged(target, content, quiet); writeErr != nil {
			return writeErr
		}
	}

	return removeOrphanColumnFiles(modelDir, wanted, quiet)
}

// groupColumnsByFile matches resolved models to the source files that declare
// them. A model that resolved no columns is dropped, because an empty Cols
// var would reference nothing; a model without a source file here was
// registered from outside the project's model directory, such as a framework
// module, and has no file to generate alongside.
func groupColumnsByFile(resolved []modelColumns, sources map[string]string) map[string][]modelColumns {
	byFile := make(map[string][]modelColumns)
	for _, m := range resolved {
		if len(m.Columns) == 0 {
			continue
		}
		file, ok := sources[m.PkgPath+"."+m.Name]
		if !ok {
			continue
		}
		byFile[file] = append(byFile[file], m)
	}
	return byFile
}

// modelPkgPath rebuilds the import path of the package declaring a model.
func modelPkgPath(m *gen.ModelInfo) string {
	dir := strings.Trim(filepath.ToSlash(m.ModelFileDir), "/")
	if dir == "" {
		return m.ModulePath
	}
	return m.ModulePath + "/" + dir
}

// columnsFileName returns the generated file that belongs to a model source
// file: model/sample/record.go becomes model/sample/record.gen.go.
func columnsFileName(source string) string {
	return strings.TrimSuffix(source, constants.ExtensionGo) + constants.SuffixGenGo
}

// renderColumnsFile builds the generated source for one model source file.
func renderColumnsFile(module string, pkgName string, source string, models []modelColumns) (string, error) {
	imports := map[string]string{constants.ImportPathTypes: "types"}
	for _, m := range models {
		for _, col := range m.Columns {
			// A TimeColumn reference carries no type argument, so the column
			// type's import would be unused in the generated file.
			if col.Time && col.TypeExpr != "" {
				continue
			}
			if col.TypePkg == "" {
				continue
			}
			dot := strings.Index(col.TypeExpr, ".")
			if dot <= 0 {
				return "", errors.Newf("model %s column %q has import %q but its type %q carries no package qualifier",
					m.Name, col.DBName, col.TypePkg, col.TypeExpr)
			}
			alias := col.TypeExpr[:dot]
			if existing, ok := imports[col.TypePkg]; ok && existing != alias {
				return "", errors.Newf("model %s column %q needs import %q as %q but it is already imported as %q",
					m.Name, col.DBName, col.TypePkg, alias, existing)
			}
			for path, other := range imports {
				if other == alias && path != col.TypePkg {
					return "", errors.Newf("model %s column %q imports %q as %q, colliding with %q; add an explicit gorm column type or rename the package",
						m.Name, col.DBName, col.TypePkg, alias, path)
				}
			}
			imports[col.TypePkg] = alias
		}
	}

	// Standard library imports go in their own group, as gofmt convention
	// expects; format.Source keeps groups but does not create them. A path is
	// standard library only when its first segment carries no dot and it does
	// not belong to the project module, whose name may also be dotless.
	stdlib := make([]string, 0, len(imports))
	external := make([]string, 0, len(imports))
	for path := range imports {
		if isStdlibImport(path, module) {
			stdlib = append(stdlib, path)
		} else {
			external = append(external, path)
		}
	}
	sort.Strings(stdlib)
	sort.Strings(external)

	var buf strings.Builder
	buf.WriteString(consts.CodeGeneratedComment())
	fmt.Fprintf(&buf, "\n// source: %s\n\npackage %s\n\nimport (\n", filepath.ToSlash(source), pkgName)
	for _, path := range stdlib {
		fmt.Fprintf(&buf, "\t%s %q\n", imports[path], path)
	}
	if len(stdlib) > 0 && len(external) > 0 {
		buf.WriteString("\n")
	}
	for _, path := range external {
		fmt.Fprintf(&buf, "\t%s %q\n", imports[path], path)
	}
	buf.WriteString(")\n")

	for _, m := range models {
		fmt.Fprintf(&buf, "\n// %sCols are the typed column references of %s.\n", m.Name, m.Name)
		fmt.Fprintf(&buf, "var %sCols = struct {\n", m.Name)
		for _, col := range m.Columns {
			fmt.Fprintf(&buf, "\t%s %s", col.GoName, columnRefType(col))
			if col.TypeExpr == "" {
				fmt.Fprintf(&buf, " // %s", col.TypeName)
			}
			buf.WriteString("\n")
		}
		buf.WriteString("}{\n")
		for _, col := range m.Columns {
			fmt.Fprintf(&buf, "\t%s: %s,\n", col.GoName, columnRefLiteral(col))
		}
		buf.WriteString("}\n")
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return "", errors.Wrapf(err, "format generated columns for %s", source)
	}
	return string(formatted), nil
}

// columnTypeParam returns the type argument for a column reference, falling
// back to any when the column type cannot be written as source.
func columnTypeParam(col columnInfo) string {
	if col.TypeExpr == "" {
		return "any"
	}
	return col.TypeExpr
}

// columnRefType returns the declared type of one generated column reference.
// Numeric and time columns get the specialized references that carry the
// aggregate functions only meaningful there; every other column, including one
// whose type cannot be written as source, gets the plain reference.
func columnRefType(col columnInfo) string {
	switch {
	case col.Time && col.TypeExpr != "":
		return "types.TimeColumn"
	case col.Numeric && col.TypeExpr != "":
		return fmt.Sprintf("types.NumericColumn[%s]", col.TypeExpr)
	default:
		return fmt.Sprintf("types.Column[%s]", columnTypeParam(col))
	}
}

// columnRefLiteral returns the constructor call initializing one generated
// column reference. Construction goes through the NewXxx constructors rather
// than composite literals because the column name field is unexported: a
// generated reference cannot be repointed at another column at run time.
func columnRefLiteral(col columnInfo) string {
	switch {
	case col.Time && col.TypeExpr != "":
		return fmt.Sprintf("types.NewTimeColumn(%q)", col.DBName)
	case col.Numeric && col.TypeExpr != "":
		return fmt.Sprintf("types.NewNumericColumn[%s](%q)", col.TypeExpr, col.DBName)
	default:
		return fmt.Sprintf("types.NewColumn[%s](%q)", columnTypeParam(col), col.DBName)
	}
}

// writeGeneratedFileIfChanged writes content only when it differs from what is
// on disk, so an unchanged model does not churn file timestamps.
func writeGeneratedFileIfChanged(path string, content string, quiet bool) error {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "read %s", path)
	}
	if err = os.WriteFile(path, []byte(content), constants.FileModeGenerated); err != nil {
		return errors.Wrapf(err, "write %s", path)
	}
	if !quiet {
		clioutput.Success("GENERATE", "%s", path)
	}
	return nil
}

// isColumnFileCandidate reports whether path can be a generated column file
// at all: it carries the generated suffix and is not one of the files another
// generation step owns, such as the model registration and apidoc files.
func isColumnFileCandidate(path string) bool {
	if !strings.HasSuffix(path, constants.SuffixGenGo) {
		return false
	}
	base := filepath.Base(path)
	return base != constants.FileModelGen && base != constants.FileAPIDocGen
}

// generatedColumnFileStubs maps every framework-owned generated column file
// under dir to a stub holding only its package clause. The inspection build
// replaces the files with these stubs through a build overlay, so resolving
// columns never depends on the previous generation's output: after a
// framework upgrade that changes the generated API, the stale files would
// otherwise fail to compile until the very run that is trying to rewrite
// them.
func generatedColumnFileStubs(dir string) (map[string]string, error) {
	stubs := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isColumnFileCandidate(path) {
			return nil
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // path comes from the model directory walk.
		if readErr != nil {
			return errors.Wrapf(readErr, "read %s", path)
		}
		// A file without the generated header is hand-written and keeps
		// participating in the build as-is.
		if !strings.HasPrefix(string(content), consts.CodeGeneratedComment()) {
			return nil
		}
		clause, parseErr := parser.ParseFile(token.NewFileSet(), path, content, parser.PackageClauseOnly)
		if parseErr != nil {
			return errors.Wrapf(parseErr, "parse package clause of %s", path)
		}
		stubs[path] = "package " + clause.Name.Name + "\n"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stubs, nil
}

// removeOrphanColumnFiles deletes generated column files whose model source no
// longer declares any model, which happens when a model is deleted, renamed,
// or moved to another file. Only files carrying the generated header are
// removed, so a hand-written file is reported instead of destroyed.
func removeOrphanColumnFiles(dir string, wanted map[string]struct{}, quiet bool) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isColumnFileCandidate(path) {
			return nil
		}
		if _, keep := wanted[path]; keep {
			return nil
		}
		content, err := os.ReadFile(path) //nolint:gosec // path comes from the model directory walk.
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}
		if !strings.HasPrefix(string(content), consts.CodeGeneratedComment()) {
			return errors.Newf("%s uses the generated file suffix but was not generated by gst; rename it", path)
		}
		// #nosec G122 -- path comes from walking the project's own model
		// directory, and only files carrying the generated header reach here.
		if err = os.Remove(path); err != nil {
			return errors.Wrapf(err, "remove orphan generated file %s", path)
		}
		if !quiet {
			clioutput.Success("REMOVE", "%s (model source is gone)", path)
		}
		return nil
	})
}

// moduleRequiresGst reports whether the project's go.mod depends on the
// framework module.
func moduleRequiresGst() (bool, error) {
	content, err := os.ReadFile("go.mod")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "read go.mod")
	}
	return strings.Contains(string(content), constants.ImportPathGst), nil
}

// columnsCacheKey hashes everything that can change the resolved columns:
//
//   - the gg binary itself, which changes whenever the framework changes how
//     columns are resolved (a local framework checkout does not show up in
//     go.mod, but reinstalling gg is what makes such a change take effect);
//   - the inspection program, including the module it targets;
//   - the module requirements, which pin the framework and gorm versions;
//   - the content of every model source file, since the models are what carry
//     the columns.
//
// File paths are deliberately excluded: renaming or moving a model file does
// not change a single column, and which file a model belongs to is resolved
// from the project scan rather than from this program, so hashing paths would
// rebuild for a rename that cannot affect the result. Moving a model to
// another package does change the result, but it also rewrites the package
// clause inside the file, which content hashing catches.
//
// A type a model refers to from outside the model directory is not covered.
// Model files are the project's declared home for those types; if a stale
// result is ever suspected, deleting the cache directory forces a fresh
// inspection.
func columnsCacheKey(program string, modelDir string) (string, error) {
	digest := sha256.New()
	digest.Write([]byte(program))

	if executable, err := os.Executable(); err == nil {
		if info, statErr := os.Stat(executable); statErr == nil {
			fmt.Fprintf(digest, "gg:%d:%d\n", info.Size(), info.ModTime().UnixNano())
		}
	}

	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", errors.Wrap(err, "read go.mod")
	}
	digest.Write(goMod)

	fileDigests := make([]string, 0)
	err = filepath.WalkDir(modelDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "generated", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, constants.ExtensionGo) ||
			strings.HasSuffix(path, constants.PatternTestFile) ||
			strings.HasSuffix(path, constants.SuffixGenGo) {
			return nil
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // path comes from walking the project's model directory.
		if readErr != nil {
			return readErr
		}
		fileDigest := sha256.Sum256(content)
		fileDigests = append(fileDigests, hex.EncodeToString(fileDigest[:]))
		return nil
	})
	if err != nil {
		return "", errors.Wrapf(err, "hash model sources under %s", modelDir)
	}
	// The per-file digests are sorted before being folded in, so the key
	// depends on the set of model sources, not on the order the walk visits
	// them nor on what the files are called.
	sort.Strings(fileDigests)
	for _, fileDigest := range fileDigests {
		fmt.Fprintln(digest, fileDigest)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// columnsCacheDir returns the per-project cache directory. Results live under
// the user cache directory rather than in the project, so a generated tree
// stays free of tooling state.
func columnsCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "resolve user cache directory")
	}
	project, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "resolve working directory")
	}
	projectDigest := sha256.Sum256([]byte(project))
	return filepath.Join(base, "gg", "columns", hex.EncodeToString(projectDigest[:8])), nil
}

// readColumnsCache returns a previously stored inspection result for the key.
func readColumnsCache(key string) ([]modelColumns, bool) {
	dir, err := columnsCacheDir()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return nil, false
	}
	var resolved []modelColumns
	if err = json.Unmarshal(content, &resolved); err != nil {
		return nil, false
	}
	return resolved, true
}

// writeColumnsCache stores an inspection result, replacing the project's
// previous entry: only the current inputs are ever worth keeping.
func writeColumnsCache(key string, resolved []modelColumns) error {
	dir, err := columnsCacheDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return errors.Wrapf(err, "create cache directory %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "read cache directory %s", dir)
	}
	for _, entry := range entries {
		if entry.Name() == key+".json" {
			continue
		}
		// #nosec G122 -- the entry comes from reading the cache directory gg owns.
		if err = os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return errors.Wrapf(err, "remove stale cache entry %s", entry.Name())
		}
	}
	content, err := json.Marshal(resolved)
	if err != nil {
		return errors.Wrap(err, "encode resolved columns")
	}
	if err = os.WriteFile(filepath.Join(dir, key+".json"), content, 0o600); err != nil {
		return errors.Wrap(err, "write column cache")
	}
	return nil
}

// isStdlibImport reports whether an import path belongs to the standard
// library rather than to a dependency or to the project itself.
func isStdlibImport(path string, module string) bool {
	if module != "" && (path == module || strings.HasPrefix(path, module+"/")) {
		return false
	}
	return !strings.Contains(strings.SplitN(path, "/", 2)[0], ".")
}

// inspectColumns compiles and runs the inspection program and decodes what it
// reports. The result travels through a file rather than stdout, because
// framework initialization writes progress lines to stdout. The build runs
// with overlay replacing the previously generated column files, so a stale
// generation never blocks the run that would refresh it.
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

	inspector := projectProgram{Content: strings.ReplaceAll(program, "{{OUTPUT}}", resultPath), Overlay: overlay}
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
