package goast

import "go/token"

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

// lineWidth is the byte length of every line of a LineSet's file.
const lineWidth = 100

// lineSetLines is how many lines a LineSet's file has.
const lineSetLines = 1 << 12

// NewLineSet adds a file of lineSetLines lines to fset and returns the
// positions of its lines, to be printed through fset.
func NewLineSet(fset *token.FileSet) *LineSet {
	file := fset.AddFile("lineset.go", -1, lineSetLines*lineWidth)
	offsets := make([]int, lineSetLines)
	for i := range offsets {
		offsets[i] = i * lineWidth
	}
	file.SetLines(offsets)
	return &LineSet{file: file}
}

// Next returns a position on the line after the one returned last, the first
// line on the first call.
func (l *LineSet) Next() token.Pos {
	l.line++
	return l.file.Pos((l.line - 1) * lineWidth)
}
