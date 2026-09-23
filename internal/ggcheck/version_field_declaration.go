package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelregistry"
	gormschema "gorm.io/gorm/schema"
)

// VersionFieldDeclaration holds model.Version declarations to the optimistic-
// locking shape.
var VersionFieldDeclaration = Check{
	Name: "Version field declaration",
	Rule: `model.Version declarations must keep the optimistic-locking shape: on database models a named field with json:",omitempty" and gorm:"not null;default:1", and on DSL Payload/Result types (plus the same-package types reachable from their fields) a json tag of exactly "version,omitempty"`,
	run:  checkVersionFieldDeclaration,
}

// checkVersionFieldDeclaration reports model.Version declarations that
// deviate from the required shape. On database models that is a NAMED
// top-level field carrying json:",omitempty" serialization and
// gorm:"not null;default:1". An embedded Version is not recognized by the
// framework and the lock silently does not engage; a missing default:1
// backfills adopted rows to zero and locks them out of Update; a json tag
// without omitempty serializes an unset version as an explicit zero the
// write paths reject, and json:"-" hides the version clients must hand back.
// "gg gen" heals the healable cases automatically — this check is the
// read-only net for code that was committed without running it.
//
// On DSL action types (the Payload/Result types referenced by Design
// methods, plus the same-package types reachable from their fields) only the
// json half applies, pinned to the exact wire form json:"version,omitempty":
// hand-written DTOs are contracts gg gen never rewrites, and a missing or
// mismatched name stays green in Go-side tests — both ends marshal the same
// struct — while real clients sending "version" never bind it. Model
// subtrees owned by copyable framework modules are skipped, as in the model
// table name and gorm tag index checks: copied module code is checked inside
// the framework.
func checkVersionFieldDeclaration(ignore gghelper.ProjectIgnore) []string {
	findings, err := collectVersionFieldFindings(ignore)
	if err != nil {
		return []string{err.Error()}
	}

	var violations []string
	for _, finding := range findings {
		relPath := gghelper.RelativePath(finding.Path)
		if finding.Embedded {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"%s\"` (an embedded Version is not recognized and the lock silently does not engage)",
				relPath, finding.Line, finding.Struct, VersionRequiredTag,
			))
			continue
		}
		if finding.JSONBlocked {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: field '%s.%s' (model.Version) carries json:\"-\"; the version must serialize so clients can hand it back",
				relPath, finding.Line, finding.Struct, finding.Field,
			))
			continue
		}
		missing := make([]string, 0, len(finding.Missing)+1)
		for _, setting := range finding.Missing {
			missing = append(missing, "gorm "+setting)
		}
		if finding.JSONMissing {
			missing = append(missing, "json omitempty")
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: field '%s.%s' (model.Version) is missing %s; run \"gg gen\" to fill the tags in — default:1 backfills existing rows to a live version when the column is added, and omitempty keeps an unset version out of marshaled request bodies",
			relPath, finding.Line, finding.Struct, finding.Field, strings.Join(missing, ", "),
		))
	}

	actionFindings, err := collectActionTypeVersionFindings(ignore)
	if err != nil {
		return append(violations, err.Error())
	}
	for _, finding := range actionFindings {
		relPath := gghelper.RelativePath(finding.Path)
		if finding.Blocked {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: field '%s.%s' (model.Version) in a DSL action type carries json:\"-\"; the version must serialize so clients can hand it back",
				relPath, finding.Line, finding.Struct, finding.Field,
			))
			continue
		}
		got := ""
		if finding.HasJSON {
			got = fmt.Sprintf(" (got json:%q)", finding.Got)
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: field '%s.%s' (model.Version) in a DSL action type must carry json:\"version,omitempty\"%s; the version handshake is wire-named \"version\", and a missing or mismatched name stays green in Go-side tests while real clients never bind it",
			relPath, finding.Line, finding.Struct, finding.Field, got,
		))
	}
	return violations
}

// collectActionTypeVersionFindings gathers the deviating model.Version
// fields of DSL action types, package by package: a Design method may
// reference a type declared in a sibling file, so files are grouped per
// directory the same way the json tag naming check groups them.
func collectActionTypeVersionFindings(ignore gghelper.ProjectIgnore) ([]actionTypeVersionFinding, error) {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil, nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return nil, fmt.Errorf("listing copyable framework modules: %w", err)
	}

	var packageDirs []string
	packageFiles := make(map[string][]string)
	walkErr := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, ggconst.DirModel, path) {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		dir := filepath.Dir(path)
		if _, seen := packageFiles[dir]; !seen {
			packageDirs = append(packageDirs, dir)
		}
		packageFiles[dir] = append(packageFiles[dir], path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking model directory: %w", walkErr)
	}

	var findings []actionTypeVersionFinding
	for _, dir := range packageDirs {
		findings = append(findings, scanPackageActionTypeVersionFields(packageFiles[dir])...)
	}
	return findings, nil
}

// VersionFieldFindings reports every model.Version declaration of a database
// model under the model directory that deviates from the required shape, the
// declarations gg gen heals by filling their tags in. Paths the project's Git
// ignore rules ignore are left out.
func VersionFieldFindings() ([]VersionFieldFinding, error) {
	return collectVersionFieldFindings(gghelper.NewProjectIgnore())
}

// collectVersionFieldFindings walks the model directory and gathers every
// deviating model.Version declaration, in walk order.
func collectVersionFieldFindings(ignore gghelper.ProjectIgnore) ([]VersionFieldFinding, error) {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil, nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return nil, fmt.Errorf("listing copyable framework modules: %w", err)
	}

	var findings []VersionFieldFinding
	walkErr := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, ggconst.DirModel, path) {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		fileFindings, err := scanVersionFieldFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking model directory: %w", walkErr)
	}
	return findings, nil
}

// Shared detection for model.Version declarations.
//
// The declaration shape is a framework contract with no user freedom: a
// NAMED top-level field carrying json:",omitempty" serialization and
// gorm:"not null;default:1" (see modelregistry.Version for why the default
// and the omitempty are load-bearing). Three layers enforce it, all reading
// the detection below: "gg gen" heals named fields by filling the tags in,
// "gg check" reports deviations read-only, and the framework panics at
// runtime on first touch as the last net. The model package is recognized
// under every name a file imports it by, a dot import included (see
// goast.ImportedNames).

// VersionRequiredTag is the exact gorm tag payload a model.Version field
// must carry.
const VersionRequiredTag = "not null;default:1"

// VersionFieldFinding describes one model.Version declaration that deviates
// from the required shape, with enough byte geometry for gg gen to rewrite
// the named-field cases in place.
type VersionFieldFinding struct {
	Path     string
	Line     int
	Struct   string
	Field    string // empty for an embedded declaration
	Embedded bool
	Missing  []string // required gorm settings absent from the tag, bare

	// JSONMissing marks a json tag that does not satisfy the omitempty
	// contract; JSONBlocked marks json:"-", which no tool may heal:
	// un-hiding a field the author silenced is a semantic decision, exactly
	// like reshaping an embedded declaration.
	JSONMissing bool
	JSONBlocked bool

	// rewrite geometry, named fields only; offsets are into the file bytes
	hasTag         bool
	tagEnd         int // one past the tag literal's closing backquote
	hasGormSection bool
	gormValueEnd   int // offset of the gorm value's closing double quote
	hasJSONSection bool
	jsonValueEnd   int // offset of the json value's closing double quote
	insertAfter    int // offset right after the field type, for a new tag
}

// TagInsertion is one insertion a finding's heal expands to: Text goes in at
// byte Offset of the file.
type TagInsertion struct {
	Offset int
	Text   string
}

// TagInsertions expands one healable finding into its byte insertions. The
// json name for a field without any json section follows gorm's naming
// strategy, so the wire name matches the column name a bare field gets.
func (finding VersionFieldFinding) TagInsertions() []TagInsertion {
	if !finding.hasTag {
		// No tag at all: both sections are missing by construction; add the
		// whole literal right after the field type.
		return []TagInsertion{{
			Offset: finding.insertAfter,
			Text:   " `json:\"" + versionJSONName(finding.Field) + ",omitempty\" gorm:\"" + VersionRequiredTag + "\"`",
		}}
	}

	var insertions []TagInsertion
	if len(finding.Missing) > 0 {
		if finding.hasGormSection {
			// Append the missing settings inside the existing gorm value.
			insertions = append(insertions, TagInsertion{finding.gormValueEnd, ";" + strings.Join(finding.Missing, ";")})
		} else {
			// Add a gorm section before the literal's closing backquote.
			insertions = append(insertions, TagInsertion{finding.tagEnd - 1, ` gorm:"` + VersionRequiredTag + `"`})
		}
	}
	if finding.JSONMissing {
		if finding.hasJSONSection {
			// Append omitempty inside the existing json value.
			insertions = append(insertions, TagInsertion{finding.jsonValueEnd, ",omitempty"})
		} else {
			insertions = append(insertions, TagInsertion{finding.tagEnd - 1, ` json:"` + versionJSONName(finding.Field) + `,omitempty"`})
		}
	}
	return insertions
}

// versionJSONName renders the wire name a healed json section uses.
func versionJSONName(fieldName string) string {
	return gormschema.NamingStrategy{}.ColumnName("", fieldName)
}

// scanVersionFieldFile reports every deviating model.Version declaration in
// one Go file. A file that does not import the framework model package is
// free of them by construction and costs one imports-only parse.
func scanVersionFieldFile(path string) ([]VersionFieldFinding, error) {
	imports, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("%s has parse error: %w", gghelper.RelativePath(path), err)
	}
	names := goast.ImportedNames(imports, ggconst.ImportPathModel, ggconst.PkgModel)
	if len(names.Qualifiers) == 0 && !names.DotImported {
		return nil, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%s has parse error: %w", gghelper.RelativePath(path), err)
	}

	var findings []VersionFieldFinding
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			// Only database models are held to the tag contract. A request
			// or response DTO may carry model.Version too — that is how a
			// client hands the version back — but it never reaches the
			// database, and a gorm tag on it would be dead weight. The
			// embedded framework base is what marks a struct as a model.
			if !slices.ContainsFunc(structType.Fields.List, func(field *ast.Field) bool { return dsl.IsModelBase(file, field) }) {
				continue
			}
			for _, field := range structType.Fields.List {
				if !names.Refers(field.Type, "Version") {
					continue
				}
				if finding, deviates := versionFieldDeviation(fset, path, typeSpec.Name.Name, field); deviates {
					findings = append(findings, finding)
				}
			}
		}
	}
	return findings, nil
}

// versionFieldDeviation classifies one model.Version field against the
// required shape and computes the rewrite geometry for gg gen.
func versionFieldDeviation(fset *token.FileSet, path, structName string, field *ast.Field) (VersionFieldFinding, bool) {
	finding := VersionFieldFinding{
		Path:   path,
		Line:   fset.Position(field.Pos()).Line,
		Struct: structName,
	}

	if len(field.Names) == 0 {
		finding.Embedded = true
		return finding, true
	}
	finding.Field = field.Names[0].Name
	finding.insertAfter = fset.Position(field.Type.End()).Offset

	rawTag := ""
	if field.Tag != nil {
		finding.hasTag = true
		finding.tagEnd = fset.Position(field.Tag.End()).Offset
		if unquoted, err := strconv.Unquote(field.Tag.Value); err == nil {
			rawTag = unquoted
		}
	}

	finding.Missing = modelregistry.VersionGormTagMissing(reflect.StructTag(rawTag))
	jsonCompliant, jsonHealable := modelregistry.VersionJSONTagState(reflect.StructTag(rawTag))
	finding.JSONMissing = !jsonCompliant
	finding.JSONBlocked = !jsonHealable
	if len(finding.Missing) == 0 && !finding.JSONMissing {
		return VersionFieldFinding{}, false
	}

	if finding.hasTag {
		literal := field.Tag.Value
		tagStart := fset.Position(field.Tag.Pos()).Offset
		if idx := strings.Index(literal, `gorm:"`); idx >= 0 {
			valueStart := idx + len(`gorm:"`)
			if rel := strings.Index(literal[valueStart:], `"`); rel >= 0 {
				finding.hasGormSection = true
				finding.gormValueEnd = tagStart + valueStart + rel
			}
		}
		if idx := strings.Index(literal, `json:"`); idx >= 0 {
			valueStart := idx + len(`json:"`)
			if rel := strings.Index(literal[valueStart:], `"`); rel >= 0 {
				finding.hasJSONSection = true
				finding.jsonValueEnd = tagStart + valueStart + rel
			}
		}
	}
	return finding, true
}

// --- DSL action types ---
//
// Request and response DTOs carry model.Version too — that is how a client
// hands the version back — but they never reach the database, so only the
// json half of the contract applies, and it applies harder: the wire name
// must be exactly "version". A missing or mismatched name is the one
// deviation Go-side integration tests cannot catch — both ends marshal the
// same struct and stay green — while real clients sending "version" never
// bind it and every save is rejected. Like the snake_case naming check, only
// types referenced by a Design method and declared in the same package are
// held to this; unreferenced DTOs may mirror external wire contracts.

// actionTypeVersionFinding describes one model.Version field of a DSL action
// type whose json tag deviates from the required json:"version,omitempty".
type actionTypeVersionFinding struct {
	Path   string
	Line   int
	Struct string
	Field  string

	// Got is the raw json tag value; HasJSON distinguishes an absent tag
	// from an empty one. Blocked marks json:"-", which mirrors the model
	// finding of the same name: no tool may un-hide a silenced field.
	Got     string
	HasJSON bool
	Blocked bool
}

// scanPackageActionTypeVersionFields reports every deviating model.Version
// field reachable from the DSL action types of one model package. Reachable
// means the Payload/Result types referenced by Design methods plus,
// transitively, the same-package types their fields point at through
// pointers, slices, arrays, map values and type aliases: nested item types
// carry per-row versions the same way top-level requests do. Structs
// embedding the framework base are left to the model-side scan, and files
// that fail to parse are skipped here because the model-side scan already
// reports the parse error.
func scanPackageActionTypeVersionFields(paths []string) []actionTypeVersionFinding {
	fset := token.NewFileSet()
	type declaration struct {
		path string
		spec *ast.TypeSpec
	}
	files := make(map[string]*ast.File, len(paths))
	namesByPath := make(map[string]goast.PackageNames, len(paths))
	covered := make(map[string]bool)
	declarations := make(map[string]declaration)
	for _, path := range paths {
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files[path] = node
		namesByPath[path] = goast.ImportedNames(node, ggconst.ImportPathModel, ggconst.PkgModel)
		for _, name := range slices.Concat(dsl.FindAllModelBase(node), dsl.FindAllModelEmpty(node)) {
			covered[name] = true
		}
		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name != nil {
					declarations[typeSpec.Name.Name] = declaration{path: path, spec: typeSpec}
				}
			}
		}
	}

	// Seed the worklist with the Payload/Result references of every Design
	// method, in file walk order so findings come out deterministically.
	var worklist []string
	seen := make(map[string]bool)
	enqueue := func(name string) {
		if _, ok := declarations[name]; ok && !covered[name] && !seen[name] {
			seen[name] = true
			worklist = append(worklist, name)
		}
	}
	for _, path := range paths {
		node, ok := files[path]
		if !ok {
			continue
		}
		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Design" || funcDecl.Recv == nil || funcDecl.Body == nil {
				continue
			}
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if _, typeExpr, ok := dslActionTypeCall(call.Fun); ok {
					if name, ok := localActionTypeName(typeExpr); ok {
						enqueue(name)
					}
				}
				return true
			})
		}
	}

	var findings []actionTypeVersionFinding
	for len(worklist) > 0 {
		decl := declarations[worklist[0]]
		worklist = worklist[1:]
		names := namesByPath[decl.path]
		switch typed := decl.spec.Type.(type) {
		case *ast.Ident:
			// A type alias or defined type hops to its target declaration.
			enqueue(typed.Name)
		case *ast.StructType:
			if typed.Fields == nil {
				continue
			}
			for _, field := range typed.Fields.List {
				if !names.Refers(field.Type, "Version") {
					for _, name := range localFieldTypeNames(field.Type) {
						enqueue(name)
					}
					continue
				}
				finding := actionTypeVersionFinding{
					Path:   decl.path,
					Line:   fset.Position(field.Pos()).Line,
					Struct: decl.spec.Name.Name,
					Field:  "Version",
				}
				if len(field.Names) > 0 {
					finding.Field = field.Names[0].Name
				}
				rawTag := ""
				if field.Tag != nil {
					if unquoted, err := strconv.Unquote(field.Tag.Value); err == nil {
						rawTag = unquoted
					}
				}
				if got, hasJSON, blocked, deviates := actionTypeVersionTagDeviation(reflect.StructTag(rawTag)); deviates {
					finding.Got, finding.HasJSON, finding.Blocked = got, hasJSON, blocked
					findings = append(findings, finding)
				}
			}
		}
	}
	return findings
}

// actionTypeVersionTagDeviation classifies the json tag of a model.Version
// field in a DSL action type against the required json:"version,omitempty":
// the wire name must be exactly "version" and omitempty must keep an unset
// version out of marshaled bodies. json:"-" is reported separately because
// no tool may heal it.
func actionTypeVersionTagDeviation(tag reflect.StructTag) (got string, hasJSON, blocked, deviates bool) {
	value, ok := tag.Lookup("json")
	if !ok {
		return "", false, false, true
	}
	name, options, _ := strings.Cut(value, ",")
	if strings.TrimSpace(name) == "-" && len(options) == 0 {
		return value, true, true, true
	}
	if strings.TrimSpace(name) != "version" {
		return value, true, false, true
	}
	for option := range strings.SplitSeq(options, ",") {
		if strings.TrimSpace(option) == "omitempty" {
			return value, true, false, false
		}
	}
	return value, true, false, true
}

// localFieldTypeNames collects the same-package type names a field type can
// reach: bare identifiers, unwrapped through pointers, slices, arrays and map
// values. Qualified names live in other packages and are out of scope.
func localFieldTypeNames(expr ast.Expr) []string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return []string{typed.Name}
	case *ast.StarExpr:
		return localFieldTypeNames(typed.X)
	case *ast.ArrayType:
		return localFieldTypeNames(typed.Elt)
	case *ast.MapType:
		return localFieldTypeNames(typed.Value)
	}
	return nil
}
