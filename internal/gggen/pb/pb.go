// Package pb generates the protobuf definitions of a gst project's gRPC
// services and the Go files serving them: for every model whose Design
// declares GRPC(), the messages its Go types encode to and the service
// exposing its actions, printed as .proto files that mirror the model
// directory under pb/; beside each, a .gen.go with the type serving the
// service, the calls of its actions, the handlers of its rpcs and the
// conversions between the messages and the Go types (see handlerFile); and,
// in every package under pb/, a pb.gen.go registering its services on the
// listener (see registrationFiles).
//
// The definitions are derived, never written by hand. Field shapes come from
// jsonshape, the same reading of the Go types the TypeScript declarations
// use, so both descriptions agree; field numbers come from the pb struct tag
// every business field carries (see Tag), while the keys of the framework's
// model base have fixed numbers (see BaseFieldNumbers). A type whose shape
// protobuf cannot express is reported as a diagnostic instead of being
// approximated, and no file is generated then.
//
// Two libraries stand in for protoc, so nothing needs it installed: the
// files are written by protoprint, the one package of jhump/protoreflect
// used, which prints the assembled descriptors as .proto source with their
// comments and layout; they are read back by bufbuild/protocompile, a
// protobuf compiler front end in pure Go that parses and links .proto
// source into descriptors, here to hold a committed file's numbers, to
// compile the definitions for the plugins that write the Go files beside
// them (see Compile) and, in tests, to compile the generated files the way
// protoc would.
//
// The files already under pb/ are the contract in force, what the clients
// were built against, so a generated file replaces one only if every field
// keeps its number and no number changes hands; the numbers and names of the
// fields a model dropped stay reserved in the new file. A change that would
// break the wire is reported, with the file to delete for accepting it on
// purpose.
package pb

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/hydroan/gst/internal/modelinfo"
)

// Tag is the struct tag naming a field's protobuf field number, as in
// `pb:"11"`.
const Tag = "pb"

// BaseFieldNumbers are the field numbers of the keys the framework's model
// base, model.Base and model.AutoBase, encodes to. They are fixed so that every
// model agrees on them; the numbers below FirstBusinessFieldNumber stay with
// the framework.
var BaseFieldNumbers = map[string]int32{
	"id":         1,
	"created_by": 2,
	"updated_by": 3,
	"created_at": 4,
	"updated_at": 5,
}

// baseFieldComments are the comments the keys of the framework's model base
// carry in every message: the base is declared outside the project, where
// the generator reads no doc comment, so each key gets a fixed one.
var baseFieldComments = map[string]string{
	"id":         "The identifier of the record, assigned when it is created.",
	"created_by": "The id of the user who created the record.",
	"updated_by": "The id of the user who last updated the record.",
	"created_at": "When the record was created.",
	"updated_at": "When the record was last updated.",
}

// FirstBusinessFieldNumber is the lowest field number a business field of a
// model embedding the framework's base may carry: 1 to 10 belong to the
// framework. A type without the base numbers its fields from 1.
const FirstBusinessFieldNumber int32 = 11

// Config describes one generation run.
type Config struct {
	// Dir is the project root the Go packages are loaded from. Diagnostic
	// file names are relative to it.
	Dir string
	// ModulePath is the module path of the project, the import path prefix
	// of every model package; its last element names the protobuf packages.
	ModulePath string
	// Models are the models of the project, their routes resolved (see
	// modelinfo.ResolveRoutes). The ones declaring GRPC() get definitions.
	Models []*modelinfo.Model
}

// File is one generated file under pb/: a .proto definition, the Go file
// Generate writes beside it with the handlers of its services and the
// conversions of its messages, the registration file, or a Go file Compile
// made of a definition.
type File struct {
	// Path is slash-separated and relative to the project root, such as
	// pb/archive/document.proto for the model file model/archive/document.go,
	// pb/archive/document.gen.go for its handlers and
	// pb/archive/document.pb.go for the messages compiled from it.
	Path string
	// Content is the source of the file.
	Content string
	// Service reports whether a definition declares a service, which is
	// what decides whether Compile writes a _grpc.pb.go beside it; false
	// for a Go file.
	Service bool
}

// Definition reports whether the file is a .proto definition, the kind
// Compile compiles, rather than a Go file.
func (f File) Definition() bool { return strings.HasSuffix(f.Path, ".proto") }

// DiagnosticsError is the error Generate returns when any diagnostic was
// reported. No file is generated then: a partial set would leave a service
// without the messages it refers to.
type DiagnosticsError struct {
	Diagnostics []jsonshape.Diagnostic
	// MissingTags lists the fields reported for want of a pb tag, each with
	// the number Generate gave it; gg gen writes them into the fields' tags
	// and generates again.
	MissingTags []MissingTag
}

// MissingTag is a field of a message that carries no pb tag, with the
// number Generate chose for it (see the numbering in messageOfStruct): the
// diagnostic reporting the field names the same number.
type MissingTag struct {
	// Path is the file declaring the field, slash-separated and relative to
	// the project root, model/record.go; Line is the line of the field.
	Path string
	Line int
	// Struct is the name of the message in its file, Record or RecordWindow
	// for the message of an unnamed struct field, and Field the Go name of
	// the field, Title.
	Struct string
	Field  string
	// Number is the field number to give the field.
	Number int32
}

// Error lists every diagnostic on a line of its own, under a line counting
// them:
//
//	1 problem(s) keep the protobuf definitions from being generated:
//	  model/sample.go:12: example.com/app/model.Sample.name: the field has no pb tag; number it pb:"11"
func (e *DiagnosticsError) Error() string {
	lines := make([]string, 0, len(e.Diagnostics)+1)
	lines = append(lines, fmt.Sprintf("%d problem(s) keep the protobuf definitions from being generated:", len(e.Diagnostics)))
	for _, d := range e.Diagnostics {
		lines = append(lines, "  "+d.String())
	}
	return strings.Join(lines, "\n")
}

// Generate loads the packages of the models declaring GRPC() and renders the
// protobuf definitions of their services and of every project type those
// reach, one .proto file per Go file the types are declared in, with the Go
// file serving each definition beside it and the registration file, sorted
// by path (see File.Definition for telling the two kinds apart). Models
// declaring no GRPC() produce no file at all.
//
// For example, the model file model/note.go of module tmpapp declaring
//
//	// Note is a note kept by the note service.
//	type Note struct {
//		// Title is the display title.
//		Title string   `json:"title" pb:"11"`
//		Tags  []string `json:"tags,omitempty" pb:"12" gorm:"-"`
//
//		model.Base
//	}
//
//	func (Note) Design() {
//		GRPC()
//		Migrate()
//		Endpoint("notes")
//		Create(func() {})
//		Get(func() {})
//	}
//
// gets the file pb/note.proto:
//
//	// Code generated by gst; DO NOT EDIT.
//
//	syntax = "proto3";
//
//	package tmpapp;
//
//	import "google/protobuf/timestamp.proto";
//
//	option go_package = "tmpapp/pb;pb";
//
//	// Note is a note kept by the note service.
//	message Note {
//	  // The identifier of the record, assigned when it is created.
//	  string id = 1;
//
//	  // The id of the user who created the record.
//	  string created_by = 2;
//
//	  // The id of the user who last updated the record.
//	  string updated_by = 3;
//
//	  // When the record was created.
//	  google.protobuf.Timestamp created_at = 4;
//
//	  // When the record was last updated.
//	  google.protobuf.Timestamp updated_at = 5;
//
//	  // Title is the display title.
//	  string title = 11;
//
//	  repeated string tags = 12;
//	}
//
//	// CreateNoteRequest is the request of NoteService.CreateNote.
//	message CreateNoteRequest {
//	  // note is the Note to create.
//	  Note note = 1;
//	}
//
//	// CreateNoteResponse is the response of NoteService.CreateNote.
//	message CreateNoteResponse {
//	  // note is the Note created.
//	  Note note = 1;
//	}
//
//	// GetNoteRequest is the request of NoteService.GetNote.
//	message GetNoteRequest {
//	  // id is the id of the Note.
//	  string id = 1;
//
//	  // expand is the associations to expand, as the _expand query parameter names them.
//	  repeated string expand = 2;
//
//	  // depth is the depth of the expansion, as the _depth query parameter.
//	  uint32 depth = 3;
//	}
//
//	// GetNoteResponse is the response of NoteService.GetNote.
//	message GetNoteResponse {
//	  // note is the Note found.
//	  Note note = 1;
//	}
//
//	// NoteService serves the actions of Note over gRPC.
//	service NoteService {
//	  // CreateNote is the Create action of Note on /api/notes.
//	  rpc CreateNote ( CreateNoteRequest ) returns ( CreateNoteResponse );
//
//	  // GetNote is the Get action of Note on /api/notes/:id.
//	  rpc GetNote ( GetNoteRequest ) returns ( GetNoteResponse );
//	}
func Generate(cfg Config) ([]File, error) {
	models := make([]*modelinfo.Model, 0)
	for _, m := range cfg.Models {
		if m.Design != nil && m.Design.GRPC {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return nil, nil
	}
	slices.SortFunc(models, func(a, b *modelinfo.Model) int {
		return cmp.Or(strings.Compare(a.ModelFilePath, b.ModelFilePath), strings.Compare(a.ModelName, b.ModelName))
	})

	roots := make([]string, 0, len(models))
	for _, m := range models {
		roots = append(roots, m.ImportPath())
	}
	project, err := jsonshape.Load(jsonshape.Config{Dir: cfg.Dir, ModulePath: cfg.ModulePath, Roots: roots})
	if err != nil {
		return nil, err
	}
	return newGenerator(cfg, project, models).generate()
}
