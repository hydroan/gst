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
