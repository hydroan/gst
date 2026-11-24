package jsonshape

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// Site locates the subject of a diagnostic.
type Site struct {
	// Subject is the Go path of the type or field, such as
	// example.com/app/model/sample.Sample.status.
	Subject string
	// Pos is where the project declares the subject, or uses the type from
	// outside the project a field belongs to. It is invalid for a subject
	// without a place of its own, such as a whole package.
	Pos token.Pos
}

// Diagnostic reports a Go type or field whose JSON shape cannot be described.
type Diagnostic struct {
	// Pos is where the project declares the subject, or uses the type from
	// outside the project a field belongs to. It is invalid for a subject
	// without a place of its own, such as a whole package.
	Pos token.Position
	// Subject is the Go path of the type or field, such as
	// example.com/app/model/sample.Sample.status.
	Subject string
	// Message tells why the subject cannot be described, and what to change.
	Message string
}

// String renders the diagnostic as "file:line: subject: message", as in
//
//	model/sample/sample.go:12: example.com/app/model/sample.class: class is reserved in TypeScript and cannot name a type; rename the Go type
//
// or as "subject: message" for a diagnostic without a position.
func (d Diagnostic) String() string {
	if d.Pos.IsValid() {
		return fmt.Sprintf("%s:%d: %s: %s", d.Pos.Filename, d.Pos.Line, d.Subject, d.Message)
	}
	return d.Subject + ": " + d.Message
}

// FieldSite locates a field of the type at s. The field's own position is
// used when the project declares it; a field of a type from outside the
// project is reported where the project uses that type.
func (p *Project) FieldSite(s Site, name string, f *types.Var) Site {
	pos := s.Pos
	if f.Pkg() != nil && p.sources[f.Pkg().Path()] != nil {
		pos = f.Pos()
	}
	return Site{Subject: s.Subject + "." + name, Pos: pos}
}

// Report records a diagnostic at s, once: a second report of the same text is
// dropped. The file name of the position is made relative to the directory
// the packages were loaded from.
func (p *Project) Report(s Site, format string, args ...any) {
	d := Diagnostic{Subject: s.Subject, Message: fmt.Sprintf(format, args...)}
	if s.Pos.IsValid() {
		d.Pos = p.fset.Position(s.Pos)
		d.Pos.Filename = p.RelativeFile(d.Pos.Filename)
	}
	if key := d.String(); !p.reported[key] {
		p.reported[key] = true
		p.diags = append(p.diags, d)
	}
}

// Diagnostics returns the diagnostics reported so far, ordered by position,
// then by subject and message.
func (p *Project) Diagnostics() []Diagnostic {
	diags := slices.Clone(p.diags)
	slices.SortFunc(diags, func(a, b Diagnostic) int {
		return cmp.Or(
			strings.Compare(a.Pos.Filename, b.Pos.Filename),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Column, b.Pos.Column),
			strings.Compare(a.Subject, b.Subject),
			strings.Compare(a.Message, b.Message),
		)
	})
	return diags
}
