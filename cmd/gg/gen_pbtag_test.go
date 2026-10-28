package main

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/pbschema"
	"github.com/stretchr/testify/require"
)

// TestRewritePBTagsWritesTheNumbersIntoTheFields holds the example of the
// rewritePBTags doc comment: the tags numbering Body 12, Tags 13, Window 14,
// From 1 and To 2 go into the fields of Draft, its layout and its trailing
// comment kept.
func TestRewritePBTagsWritesTheNumbersIntoTheFields(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("model", "draft.go")
	writeProjectFile(t, path, strings.ReplaceAll(`package sample

import "github.com/hydroan/gst/model"

type Draft struct {
	Title  string   'json:"title" pb:"11"'
	Body   string   'json:"body"'
	Tags   []string 'json:"tags,omitempty" gorm:"-"' // trailing comment
	Window struct {
		From string 'json:"from"'
		To   string
	} 'json:"window" gorm:"-"'

	model.Base
}
`, "'", "`"))
	tags := []pbschema.MissingTag{
		{Path: "model/draft.go", Line: 7, Struct: "Draft", Field: "Body", Number: 12},
		{Path: "model/draft.go", Line: 8, Struct: "Draft", Field: "Tags", Number: 13},
		{Path: "model/draft.go", Line: 9, Struct: "Draft", Field: "Window", Number: 14},
		{Path: "model/draft.go", Line: 10, Struct: "DraftWindow", Field: "From", Number: 1},
		{Path: "model/draft.go", Line: 11, Struct: "DraftWindow", Field: "To", Number: 2},
	}

	require.NoError(t, rewritePBTags(path, tags))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.ReplaceAll(`package sample

import "github.com/hydroan/gst/model"

type Draft struct {
	Title  string   'json:"title" pb:"11"'
	Body   string   'json:"body" pb:"12"'
	Tags   []string 'json:"tags,omitempty" gorm:"-" pb:"13"' // trailing comment
	Window struct {
		From string 'json:"from" pb:"1"'
		To   string 'pb:"2"'
	} 'json:"window" gorm:"-" pb:"14"'

	model.Base
}
`, "'", "`"), string(got))
}

// TestWithPBTagAppendsTheEntryInsideTheLiteral holds the examples of the
// withPBTag doc comment: a new raw literal for a field without a tag, and the
// entry appended inside the literal otherwise, raw or interpreted.
func TestWithPBTagAppendsTheEntryInsideTheLiteral(t *testing.T) {
	tests := []struct {
		name string
		tag  *ast.BasicLit
		want string
	}{
		{name: "no_tag", want: "`pb:\"11\"`"},
		{name: "raw", tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\"title\"`"}, want: "`json:\"title\" pb:\"11\"`"},
		{name: "interpreted", tag: &ast.BasicLit{Kind: token.STRING, Value: `"json:\"title\""`}, want: `"json:\"title\" pb:\"11\""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, withPBTag(tt.tag, 11).Value)
		})
	}
}
