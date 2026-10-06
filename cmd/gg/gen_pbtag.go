package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
)

// fillPBTags numbers the fields of the models served over gRPC that carry no
// pb tag before the checks run: it derives the protobuf definitions the way
// gg gen does (see pb.Generate) and, for every field the generator reports
// for want of a tag, writes the number the generator chose into the field's
// tag (see rewritePBTags), so that a model gets its numbers the first time
// gg gen sees it and keeps them from then on, and the definitions check
// passes the very command that fixes them. The generator picks the numbers
// because it alone knows what the committed definitions reserve. Any other
// diagnostic stops the run here: alone, before a file is written, so that a
// model the generator refuses leaves the project as it was; beside missing
// tags, after those are written, the second derivation reporting it, so the
// numbers a model was given stay with it. A project whose packages cannot
// be loaded is left to the checks to report.
//
// It returns the files the derivation produced: the generation below
// reuses them instead of loading and type checking the model packages a
// second time, which nothing between the two changes, the model files and
// the tags they carry being what the definitions are derived from. After a
// tag was written the definitions are derived again, from the files as
// they now are, and what that derivation reports stops the run too.
func fillPBTags(quiet bool, ignore gghelper.ProjectIgnore) ([]pb.File, error) {
	if !gghelper.FileExists(ggconst.DirModel) {
		return nil, nil
	}
	scanned, err := scanModels(true, ignore)
	if err != nil {
		return nil, err
	}
	files, err := pb.Generate(pb.Config{Dir: ".", ModulePath: module, Models: scanned.models})
	if err == nil {
		return files, nil
	}
	var diagnostics *pb.DiagnosticsError
	if !errors.As(err, &diagnostics) {
		return nil, nil
	}
	if len(diagnostics.MissingTags) == 0 {
		return nil, err
	}
	tags, err := oneTagPerField(diagnostics.MissingTags)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string][]pb.MissingTag)
	for _, tag := range tags {
		byPath[tag.Path] = append(byPath[tag.Path], tag)
	}
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		if err := rewritePBTags(path, byPath[path]); err != nil {
			return nil, err
		}
		if !quiet {
			for _, tag := range byPath[path] {
				clioutput.Success("FIX", "%s: numbered %s.%s pb:%q", path, tag.Struct, tag.Field, strconv.Itoa(int(tag.Number)))
			}
		}
	}
	return pb.Generate(pb.Config{Dir: ".", ModulePath: module, Models: scanned.models})
}

// oneTagPerField returns tags with the ones numbering a field several
// times, once per message embedding its struct, folded into one: the field
// gets the number when the messages agree on it, and the run fails when
// they do not, since a number one message leaves free may be taken in
// another, so the field has to be numbered by hand.
func oneTagPerField(tags []pb.MissingTag) ([]pb.MissingTag, error) {
	type field struct {
		path string
		line int
		name string
	}
	folded := make([]pb.MissingTag, 0, len(tags))
	seen := make(map[field]int)
	for _, tag := range tags {
		key := field{tag.Path, tag.Line, tag.Field}
		at, ok := seen[key]
		if !ok {
			seen[key] = len(folded)
			folded = append(folded, tag)
			continue
		}
		if folded[at].Number != tag.Number {
			return nil, errors.Newf("%s:%d: %s is embedded in messages that would number it %d and %d; number it by hand with a pb tag each of them leaves free",
				tag.Path, tag.Line, tag.Field, folded[at].Number, tag.Number)
		}
	}
	return folded, nil
}

// rewritePBTags writes the numbers of tags into the fields of the file at
// path: it parses the file, finds every field by its line and name, adds
// pb:"N" to its tag (see withPBTag) and prints the file back through
// go/format, which keeps its comments and layout. A field sharing its
// declaration with another, X, Y int32, is refused, since one tag would
// number both. For the tags numbering Body 12, Tags 13, Window 14, From 1
// and To 2 in
//
//	type Draft struct {
//		Title  string   `json:"title" pb:"11"`
//		Body   string   `json:"body"`
//		Tags   []string `json:"tags,omitempty" gorm:"-"` // trailing comment
//		Window struct {
//			From string `json:"from"`
//			To   string
//		} `json:"window" gorm:"-"`
//
//		model.Base
//	}
//
// it writes
//
//	type Draft struct {
//		Title  string   `json:"title" pb:"11"`
//		Body   string   `json:"body" pb:"12"`
//		Tags   []string `json:"tags,omitempty" gorm:"-" pb:"13"` // trailing comment
//		Window struct {
//			From string `json:"from" pb:"1"`
//			To   string `pb:"2"`
//		} `json:"window" gorm:"-" pb:"14"`
//
//		model.Base
//	}
func rewritePBTags(path string, tags []pb.MissingTag) error {
	safePath, err := pathUnderRoot(path, ".")
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
	for _, tag := range tags {
		field := fieldAtLine(fset, file, tag.Line, tag.Field)
		if field == nil {
			return errors.Newf("%s:%d: field %s of %s not found for the pb tag rewrite", path, tag.Line, tag.Field, tag.Struct)
		}
		if len(field.Names) > 1 {
			names := make([]string, 0, len(field.Names))
			for _, id := range field.Names {
				names = append(names, id.Name)
			}
			return errors.Newf("%s:%d: %s share one declaration, which one pb tag would number alike; declare each field on a line of its own, then run gg gen again", path, tag.Line, strings.Join(names, ", "))
		}
		field.Tag = withPBTag(field.Tag, tag.Number)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return errors.Wrapf(err, "%s: pb tag rewrite produced unparsable code", path)
	}
	return os.WriteFile(safePath, buf.Bytes(), stat.Mode().Perm())
}

// fieldAtLine returns the field of file declared under name on line, nil
// when the file declares none: the field the generator numbered, wherever
// its struct is nested.
func fieldAtLine(fset *token.FileSet, file *ast.File, line int, name string) *ast.Field {
	var found *ast.Field
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if found != nil || !ok {
			return found == nil
		}
		for _, id := range field.Names {
			if id.Name == name && fset.Position(id.NamePos).Line == line {
				found = field
				return false
			}
		}
		return true
	})
	return found
}

// withPBTag returns the tag literal tag with pb:"number" added: a new raw
// literal for a field without a tag, the entry appended after the others
// inside the literal otherwise, whether it is raw or interpreted:
// `json:"title" pb:"11"` for `json:"title"`, and "json:\"title\" pb:\"11\""
// for "json:\"title\"".
func withPBTag(tag *ast.BasicLit, number int32) *ast.BasicLit {
	entry := fmt.Sprintf(`pb:"%d"`, number)
	if tag == nil {
		return &ast.BasicLit{Kind: token.STRING, Value: "`" + entry + "`"}
	}
	content, err := strconv.Unquote(tag.Value)
	if err != nil {
		// A literal go/parser accepted always unquotes; a raw one that does
		// not is kept as it is, with the entry inside its backquotes.
		content = strings.Trim(tag.Value, "`")
	}
	if content != "" {
		content += " "
	}
	content += entry
	value := "`" + content + "`"
	if !strings.HasPrefix(tag.Value, "`") {
		value = strconv.Quote(content)
	}
	return &ast.BasicLit{Kind: token.STRING, Value: value, ValuePos: tag.ValuePos}
}
