package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/modelregistry"
)

// Shared detection for model.Version declarations.
//
// The declaration shape is a framework contract with no user freedom: a
// NAMED top-level field carrying json:",omitempty" serialization and
// gorm:"not null;default:1" (see modelregistry.Version for why the default
// and the omitempty are load-bearing). Three layers enforce it, all reading
// the detection below: "gg gen" heals named fields by filling the tags in,
// "gg check" reports deviations read-only, and the framework panics at
// runtime on first touch as the last net. Dot imports of the model package
// are not resolved here; the runtime layer still catches those.

// gstModelImportPath is the framework package whose Version type opts a
// model into optimistic locking.
const gstModelImportPath = "github.com/hydroan/gst/model"

// versionRequiredTag is the exact gorm tag payload a model.Version field
// must carry.
const versionRequiredTag = "not null;default:1"

// versionFieldFinding describes one model.Version declaration that deviates
// from the required shape, with enough byte geometry for gg gen to rewrite
// the named-field cases in place.
type versionFieldFinding struct {
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

// scanVersionFieldFile reports every deviating model.Version declaration in
// one Go file. A file that does not import the framework model package is
// free of them by construction and costs one imports-only parse.
func scanVersionFieldFile(path string) ([]versionFieldFinding, error) {
	aliases, ok, err := modelImportAliases(path)
	if err != nil || !ok {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%s has parse error: %w", relativePath(path), err)
	}

	var findings []versionFieldFinding
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
			if !structEmbedsModelBase(structType, aliases) {
				continue
			}
			for _, field := range structType.Fields.List {
				if !isModelVersionType(field.Type, aliases) {
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

// structEmbedsModelBase reports whether the struct embeds the framework's
// model.Base or model.AutoBase under one of the file's import aliases, which
// is what marks it as a database model rather than a request/response type.
func structEmbedsModelBase(structType *ast.StructType, aliases []string) bool {
	for _, field := range structType.Fields.List {
		if len(field.Names) > 0 {
			continue
		}
		selector, ok := field.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil {
			continue
		}
		if selector.Sel.Name != "Base" && selector.Sel.Name != "AutoBase" {
			continue
		}
		if ident, ok := selector.X.(*ast.Ident); ok && slices.Contains(aliases, ident.Name) {
			return true
		}
	}
	return false
}

// versionFieldDeviation classifies one model.Version field against the
// required shape and computes the rewrite geometry for gg gen.
func versionFieldDeviation(fset *token.FileSet, path, structName string, field *ast.Field) (versionFieldFinding, bool) {
	finding := versionFieldFinding{
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
		return versionFieldFinding{}, false
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
	aliasesByPath := make(map[string][]string, len(paths))
	covered := make(map[string]bool)
	declarations := make(map[string]declaration)
	for _, path := range paths {
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files[path] = node
		aliasesByPath[path] = modelAliasesOf(node)
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
		aliases := aliasesByPath[decl.path]
		switch typed := decl.spec.Type.(type) {
		case *ast.Ident:
			// A type alias or defined type hops to its target declaration.
			enqueue(typed.Name)
		case *ast.StructType:
			if typed.Fields == nil {
				continue
			}
			for _, field := range typed.Fields.List {
				if !isModelVersionType(field.Type, aliases) {
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

// modelAliasesOf returns the local names under which an already parsed file
// imports the framework model package.
func modelAliasesOf(file *ast.File) []string {
	var aliases []string
	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"`+gstModelImportPath+`"` {
			continue
		}
		switch {
		case imp.Name == nil:
			aliases = append(aliases, "model")
		case imp.Name.Name != "_" && imp.Name.Name != ".":
			aliases = append(aliases, imp.Name.Name)
		}
	}
	return aliases
}

// modelImportAliases returns the local names under which path imports the
// framework model package. Parsing imports only keeps files that never
// mention the package cheap to skip.
func modelImportAliases(path string) (aliases []string, found bool, err error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, false, fmt.Errorf("%s has parse error: %w", relativePath(path), err)
	}
	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"`+gstModelImportPath+`"` {
			continue
		}
		found = true
		switch {
		case imp.Name == nil:
			aliases = append(aliases, "model")
		case imp.Name.Name != "_" && imp.Name.Name != ".":
			aliases = append(aliases, imp.Name.Name)
		}
	}
	return aliases, found, nil
}

// isModelVersionType reports whether expr is a reference to the framework
// model package's Version type under one of the file's import aliases.
func isModelVersionType(expr ast.Expr, aliases []string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != "Version" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return slices.Contains(aliases, ident.Name)
}
