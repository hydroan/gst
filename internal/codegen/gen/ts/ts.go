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
	// RootPath is the import path of the package tree the output mirrors: the
	// model directory, which is therefore not repeated in every output path. A
	// type declared outside it, such as one a model field borrows from another
	// package of the project, keeps its path relative to ModulePath.
	RootPath string
	// AppName names the prelude file, so a frontend holding the copied
	// directory can tell whose API these types describe. The framework name is
	// used when the project configured no name.
	AppName string
	// Roots are the types the routes send and receive. Generation declares
	// them and every project type they reach.
	Roots []TypeRef
}

// TypeRef names a type declared at package scope, such as
// {PkgPath: "example.com/app/model/sample", Name: "Sample"}.
type TypeRef struct {
	PkgPath string // import path of the package declaring the type
	Name    string // name of the type
}

// File is one generated TypeScript file.
type File struct {
	// Path is slash-separated and relative to the output directory, such as
	// sample.ts for the model package sample and pkg/notifier.ts for a
	// package outside the model directory (see Config.RootPath).
	Path string
	// Content is the TypeScript source of the file.
	Content string
}

// Diagnostic reports a Go type or field whose JSON shape the generator cannot
// describe.
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

// DiagnosticsError is the error Generate returns when any diagnostic was
// reported. No file is generated then: a partial set would hand the frontend
// declarations that refer to missing ones.
type DiagnosticsError struct {
	Diagnostics []Diagnostic
}

// Error lists every diagnostic on a line of its own, under a line counting
// them:
//
//	1 type(s) cannot be described in TypeScript:
//	  model/sample/sample.go:12: example.com/app/model/sample.class: class is reserved in TypeScript and cannot name a type; rename the Go type
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
// sorted by path. Alongside them comes the prelude, named after the
// application, declaring the JSON the framework wraps around those types.
// Roots without a single type -- a project with no routes -- produce no file at
// all, so the output can be kept in step with the models by deleting what a run
// did not write.
//
// For example, a package record reached from the roots that declares
//
//	// Record is an item a sample points at.
//	type Record struct {
//		// Title is the display title.
//		Title    string  `json:"title"`
//		Parent   *Record `json:"parent"`
//		Progress State   `json:"state"`
//	}
//
//	// State is the progress of a record. Its first constant is the zero value.
//	type State int
//
//	const (
//		StateOpen   State = iota // StateOpen marks a record in progress.
//		StateClosed              // StateClosed marks a finished record.
//	)
//
// gets the file record.ts:
//
//	// Code generated by gst; DO NOT EDIT.
//
//	/** Record is an item a sample points at. */
//	export interface Record {
//	  /** Title is the display title. */
//	  title: string;
//	  parent?: Record | null;
//	  state: State;
//	}
//
//	/**
//	 * State is the progress of a record. Its first constant is the zero value.
//	 *
//	 * - 0: StateOpen marks a record in progress.
//	 * - 1: StateClosed marks a finished record.
//	 */
//	export type State = 0 | 1;
func Generate(cfg Config) ([]File, error) {
	pkgs, err := load(cfg)
	if err != nil {
		return nil, err
	}
	return newGenerator(cfg, pkgs).generate()
}
