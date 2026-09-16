// Package ts generates the TypeScript declarations of the Go types a gst
// project's API routes send and receive.
//
// The declarations describe JSON the way encoding/json writes and reads it,
// the codec the framework's response envelope and request binding use. The
// output is types only -- interfaces, type aliases and type-only imports, all
// erased by a TypeScript compiler -- so the files can be copied into any
// frontend, whatever HTTP client or UI library it uses.
//
// Go types declared under the project module path get a declaration each, in
// one file per Go package; a type of any other package is spelled out where it
// is used. A type whose JSON shape cannot be read off its declaration, such as
// one with encoding methods of its own, is reported as a diagnostic instead of
// being widened to an unchecked type.
package ts

import (
	"fmt"
	"go/token"
	"strings"
)

// Config describes one generation run.
type Config struct {
	// Dir is the directory the Go packages are loaded from, normally the root
	// of the project module. Diagnostic file names are relative to it.
	Dir string
	// ModulePath is the import path prefix of the packages whose types get
	// declarations of their own.
	ModulePath string
	// Roots are the types the routes send and receive. Generation declares
	// them and every project type they reach.
	Roots []TypeRef
}

// TypeRef names a type declared at package scope.
type TypeRef struct {
	PkgPath string
	Name    string
}

// File is one generated TypeScript file.
type File struct {
	// Path is slash-separated and relative to the output directory.
	Path    string
	Content string
}

// Diagnostic reports a Go type or field whose JSON shape the generator cannot
// describe.
type Diagnostic struct {
	Pos token.Position
	// Subject is the Go path of the type or field, such as
	// example.com/app/model/sample.Sample.status.
	Subject string
	Message string
}

// String renders the diagnostic as "file:line: subject: message".
func (d Diagnostic) String() string {
	if d.Pos.IsValid() {
		return fmt.Sprintf("%s:%d: %s: %s", d.Pos.Filename, d.Pos.Line, d.Subject, d.Message)
	}
	return d.Subject + ": " + d.Message
}

// DiagnosticsError is the error Generate returns when any diagnostic was
// reported. No file is generated then: a partial set would hand the frontend
// declarations that refer to missing ones.
type DiagnosticsError struct {
	Diagnostics []Diagnostic
}

// Error lists every diagnostic on a line of its own.
func (e *DiagnosticsError) Error() string {
	lines := make([]string, 0, len(e.Diagnostics)+1)
	lines = append(lines, fmt.Sprintf("%d type(s) cannot be described in TypeScript:", len(e.Diagnostics)))
	for _, d := range e.Diagnostics {
		lines = append(lines, "  "+d.String())
	}
	return strings.Join(lines, "\n")
}

// Generate loads the packages of the root types and renders the TypeScript
// declarations of every project type the roots reach, one file per Go package,
// sorted by path. The prelude file gst.ts, declaring the JSON the framework
// wraps around those types, is always part of the result.
func Generate(cfg Config) ([]File, error) {
	pkgs, err := load(cfg)
	if err != nil {
		return nil, err
	}
	return newGenerator(cfg, pkgs).generate()
}
