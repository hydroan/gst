// Package pbschema derives the protobuf definitions of a gst project's gRPC
// services: for every model whose Design declares GRPC(), the messages its
// Go types encode to and the service exposing its actions, as the
// descriptors of the .proto files that mirror the model directory under pb/,
// held to the files already committed there (see Derive). The generator in
// gggen/pb prints them and writes the Go files serving them; gg check
// derives them alone, to report what generation would refuse.
//
// The definitions are derived, never written by hand. Field shapes come from
// jsonshape, the same reading of the Go types the TypeScript declarations
// use, so both descriptions agree; field numbers come from the pb struct tag
// every business field carries (see pbTag), while the keys of the framework's
// model base and tenant scope have fixed numbers (see frameworkFields). A
// type whose shape protobuf cannot express is reported as a diagnostic
// instead of being approximated, and nothing is derived then.
//
// The files already under pb/ are the contract in force, what the clients
// were built against, so a derived file replaces one only if every field
// keeps its number and no number changes hands; the numbers and names of the
// fields a model dropped stay reserved in the new file. A change that would
// break the wire is reported, with the edit of the committed file that
// accepts it on purpose while keeping what the file reserves (see
// reconcile).
package pbschema

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/hydroan/gst/internal/modelinfo"
)

// pbTag is the struct tag naming a field's protobuf field number, as in
// `pb:"11"`.
const pbTag = "pb"

// frameworkField is a key a struct of the framework encodes to, one a model
// embeds: its field number is fixed, so that every model agrees on it, and
// so is its comment, the struct being declared outside the project, where
// the derivation reads no doc comment.
type frameworkField struct {
	number  int32
	pkg     string // the path of the package declaring the field
	comment string
	// outputOnly marks the key of a value the server writes alone, which
	// the conversion of a message into the model leaves out (see
	// FieldConversion.OutputOnly).
	outputOnly bool
}

// frameworkFields are the keys of the framework's model base, model.Base and
// model.AutoBase, and of its tenant scope, tenant.Scope; the numbers below
// firstBusinessFieldNumber stay with the framework. The tenant is stamped
// from the caller when the record is created and never written again, so a
// value a message carries for it could never take effect.
var frameworkFields = map[string]frameworkField{
	"id":         {number: 1, pkg: modelRegistryPath, comment: "The identifier of the record, assigned when it is created."},
	"created_by": {number: 2, pkg: modelRegistryPath, comment: "The id of the user who created the record."},
	"updated_by": {number: 3, pkg: modelRegistryPath, comment: "The id of the user who last updated the record."},
	"created_at": {number: 4, pkg: modelRegistryPath, comment: "When the record was created."},
	"updated_at": {number: 5, pkg: modelRegistryPath, comment: "When the record was last updated."},
	"tenant_id":  {number: 6, pkg: tenantPath, comment: "The tenant the record belongs to, stamped from the caller when the record is created.", outputOnly: true},
}

// firstBusinessFieldNumber is the lowest field number a business field of a
// model carrying the framework's keys may carry: 1 to 10 belong to the
// framework. A type without them numbers its fields from 1.
const firstBusinessFieldNumber int32 = 11

// Config describes one run: the derivation, and the generation built on it.
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
	// Overlay is the build overlay the model packages are type-checked
	// through, see jsonshape.Config.Overlay: the generated files of the
	// previous run stubbed out, so a registration or column file still naming
	// a model deleted since does not fail the load.
	Overlay map[string][]byte
}

// DiagnosticsError is the error Derive returns when any diagnostic was
// reported, and the generator passes on. Nothing is derived then: a partial
// set would leave a service without the messages it refers to.
type DiagnosticsError struct {
	Diagnostics []jsonshape.Diagnostic
	// MissingTags lists the fields reported for want of a pb tag, each with
	// the number Derive gave it; gg gen writes them into the fields' tags
	// and generates again.
	MissingTags []MissingTag
}

// MissingTag is a field of a message that carries no pb tag, with the
// number Derive chose for it (see the numbering in messageOfStruct): the
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

// Derive loads the packages of the models declaring GRPC() and derives the
// protobuf definitions of their services and of every project type those
// reach, one file per Go file the types are declared in, held to the files
// committed under pb/. It returns a Schema with no file when no model declares
// GRPC(), and a *DiagnosticsError, with no definitions at all, when any
// diagnostic was reported.
func Derive(cfg Config) (*Schema, error) {
	models := make([]*modelinfo.Model, 0)
	for _, m := range cfg.Models {
		if m.Design != nil && m.Design.GRPC {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return newSchema(cfg, nil, nil), nil
	}
	slices.SortFunc(models, func(a, b *modelinfo.Model) int {
		return cmp.Or(strings.Compare(a.ModelFilePath, b.ModelFilePath), strings.Compare(a.ModelName, b.ModelName))
	})

	roots := make([]string, 0, len(models))
	for _, m := range models {
		roots = append(roots, m.ImportPath())
	}
	project, err := jsonshape.Load(jsonshape.Config{Dir: cfg.Dir, ModulePath: cfg.ModulePath, Roots: roots, Overlay: cfg.Overlay})
	if err != nil {
		return nil, err
	}
	s := newSchema(cfg, project, models)
	if err := s.derive(); err != nil {
		return nil, err
	}
	return s, nil
}
