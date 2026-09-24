package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// fillVersionFieldTags rewrites named model.Version fields under the model
// directory to carry the required json and gorm tags, printing each fix. It
// runs BEFORE the project checks so a deviating declaration heals instead of
// failing the check gen itself runs first. The tag shape is a framework
// contract with no user freedom, which is what makes the rewrite legitimate:
// like the service skeleton correction, gen is normalizing a framework-owned
// declaration, not editing user logic.
//
// Two shapes cannot be healed and abort generation with the guidance the
// check gives: an embedded declaration (the required change is the field
// shape itself) and json:"-" (un-hiding a field its author silenced is a
// semantic decision no tool should make).
func fillVersionFieldTags(quiet bool) error {
	findings, err := ggcheck.VersionFieldFindings()
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return nil
	}

	byFile := make(map[string][]ggcheck.VersionFieldFinding)
	for _, finding := range findings {
		if finding.Embedded {
			return fmt.Errorf(
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field (Version model.Version `json:\"version,omitempty\" gorm:\"%s\"`) — gen cannot heal a field shape",
				gghelper.RelativePath(finding.Path), finding.Line, finding.Struct, ggcheck.VersionRequiredTag)
		}
		if finding.JSONBlocked {
			return fmt.Errorf(
				"%s:%d: field '%s.%s' (model.Version) carries json:\"-\"; the version must serialize so clients can hand it back — gen cannot un-hide a field its author silenced",
				gghelper.RelativePath(finding.Path), finding.Line, finding.Struct, finding.Field)
		}
		byFile[finding.Path] = append(byFile[finding.Path], finding)
	}

	for path, fileFindings := range byFile {
		if err := rewriteVersionFieldTags(path, fileFindings); err != nil {
			return err
		}
		if !quiet {
			for _, finding := range fileFindings {
				fixes := make([]string, 0, 2)
				if len(finding.Missing) > 0 {
					fixes = append(fixes, `gorm:"`+strings.Join(finding.Missing, ";")+`"`)
				}
				if finding.JSONMissing {
					fixes = append(fixes, `json:",omitempty"`)
				}
				clioutput.Success("FIX", "%s: filled %s on %s.%s",
					gghelper.RelativePath(path), strings.Join(fixes, " and "), finding.Struct, finding.Field)
			}
		}
	}
	return nil
}

// rewriteVersionFieldTags heals the tags of the findings of one file: it
// parses the file, gives each found field its healed tag literal, and prints
// the file back through go/format, which keeps its comments and layout. For
// the findings of
//
//	type Bare struct {
//		Version model.Version // trailing comment
//	}
//
//	type Partial struct {
//		Version model.Version `json:"version" gorm:"not null"`
//	}
//
// it writes
//
//	type Bare struct {
//		Version model.Version `json:"version,omitempty" gorm:"not null;default:1"` // trailing comment
//	}
//
//	type Partial struct {
//		Version model.Version `json:"version,omitempty" gorm:"not null;default:1"`
//	}
func rewriteVersionFieldTags(path string, findings []ggcheck.VersionFieldFinding) error {
	safePath, err := pathUnderRoot(path, ggconst.DirModel)
	if err != nil {
		return err
	}
	stat, err := os.Stat(safePath)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, safePath, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	for _, finding := range findings {
		field := versionField(file, finding)
		if field == nil {
			return fmt.Errorf("%s:%d: field '%s.%s' not found for the version tag rewrite", gghelper.RelativePath(path), finding.Line, finding.Struct, finding.Field)
		}
		tag, err := healedVersionTag(fset, field.Tag, finding)
		if err != nil {
			return fmt.Errorf("%s: %w", gghelper.RelativePath(path), err)
		}
		field.Tag = tag
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return fmt.Errorf("%s: version tag rewrite produced unparsable code: %w", gghelper.RelativePath(path), err)
	}
	return os.WriteFile(safePath, buf.Bytes(), stat.Mode().Perm())
}

// versionField returns the field of file the finding names, the field
// finding.Field of the struct finding.Struct, or nil when the file declares
// no such field.
func versionField(file *ast.File, finding ggcheck.VersionFieldFinding) *ast.Field {
	spec := goast.FindStructTypeSpec(file, finding.Struct)
	if spec == nil {
		return nil
	}
	structType, ok := spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			if name.Name == finding.Field {
				return field
			}
		}
	}
	return nil
}

// healedVersionTag returns the tag literal of a version field after the
// finding's insertions (see ggcheck.VersionFieldFinding.TagInsertions): a
// field without a tag gets the whole literal, and a tagged field gets the
// insertions applied inside its literal, bottom-up so earlier offsets stay
// valid, which lands one finding's same-offset insertions in declaration
// order. An insertion outside the literal is an error: the finding no longer
// describes the file.
func healedVersionTag(fset *token.FileSet, tag *ast.BasicLit, finding ggcheck.VersionFieldFinding) (*ast.BasicLit, error) {
	insertions := finding.TagInsertions()
	if tag == nil {
		return &ast.BasicLit{Kind: token.STRING, Value: strings.TrimPrefix(insertions[0].Text, " ")}, nil
	}
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].Offset > insertions[j].Offset })
	start := fset.Position(tag.ValuePos).Offset
	value := tag.Value
	for _, insertion := range insertions {
		at := insertion.Offset - start
		if at < 0 || at > len(value) {
			return nil, fmt.Errorf("version tag rewrite offset out of range for %s.%s", finding.Struct, finding.Field)
		}
		value = value[:at] + insertion.Text + value[at:]
	}
	return &ast.BasicLit{Kind: token.STRING, Value: value, ValuePos: tag.ValuePos}, nil
}
