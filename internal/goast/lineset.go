package goast

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/hydroan/gst/internal/consts"
)

// LineSet hands out positions on successive lines of a fabricated file, for
// the nodes of a syntax tree built from scratch that go/printer lays out by
// their lines: the elements of a composite literal break across lines, and a
// comment starts a line of its own, only where their positions say so. Every
// line is 100 bytes wide, so a position up to 99 bytes past the one Next
// returned stays on that line: a node placed there prints on the line, after
// whatever the printer put before it.
type LineSet struct {
	file *token.File
	line int
}

// lineWidth is the byte length of every line of a LineSet's file, and
// lineSetSize the size of the file, which bounds the lines it can hold.
const (
	lineWidth   = 100
	lineSetSize = 1 << 30
)

// NewLineSet adds a file to fset and returns the positions of its lines, to
// be printed through fset. The file grows a line at a time as Next is called.
func NewLineSet(fset *token.FileSet) *LineSet {
	return &LineSet{file: fset.AddFile("lineset.go", -1, lineSetSize)}
}

// Next returns a position on the line after the one returned last, the first
// line on the first call. It panics once the file has no room for another
// line, rather than returning a position the file would fold onto its last
// line and print in the wrong place.
func (l *LineSet) Next() token.Pos {
	offset := l.line * lineWidth
	if offset+lineWidth > lineSetSize {
		panic(fmt.Sprintf("goast: LineSet has no room for line %d", l.line+1))
	}
	if l.line > 0 {
		l.file.AddLine(offset)
	}
	l.line++
	return l.file.Pos(offset)
}

// GeneratedHeader puts the generated-code header above the package clause of
// f, a blank line between the two, and returns the FileSet f is printed
// through: the header comment takes the first line of a fabricated file and
// the package clause the third, so go/printer lays them out that way. A
// comment right above the clause would be the package's doc, and a project's
// own package doc beside it a second one.
func GeneratedHeader(f *ast.File) *token.FileSet {
	fset := token.NewFileSet()
	lines := NewLineSet(fset)
	f.Comments = []*ast.CommentGroup{{List: []*ast.Comment{{Slash: lines.Next(), Text: consts.CodeGeneratedComment()}}}}
	lines.Next()
	f.Package = lines.Next()
	f.Name.NamePos = f.Package
	return fset
}
