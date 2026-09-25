package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// TestGenRunWritesTheProtobufDefinitionsOfGRPCModels holds the .proto files
// gg gen writes for the models declaring GRPC() against testdata/pb/golden;
// run it with -update to rewrite them. There is one file per model file
// under pb/, mirroring the model directory, with the model's message, the
// messages of its standard actions, the Go types of its custom actions and
// its service; a model without GRPC() gets no file. The files are compiled
// the way protoc compiles them as well. They hold, byte for byte, the
// examples the doc comments of the pb package show: pb.Generate's whole
// note.proto, and the excerpts of buildMessage, fieldTypeOf, fieldComment,
// declareService, rpcMessages, customRequest, customResponse,
// standardMessages, queryFields and descriptor.
func TestGenRunWritesTheProtobufDefinitionsOfGRPCModels(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{
		"model/record.go":      protobufRecordModel,
		"model/record/item.go": protobufItemModel,
		"model/report.go":      protobufReportModel,
		"model/plain.go":       protobufPlainModel,
		"model/note.go":        protobufNoteModel,
	})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	got := readProtos(t, filepath.Join(projectDir, "pb"))
	golden := filepath.Join(frameworkRepoRoot(t), "cmd", "gg", "testdata", "pb", "golden")
	if *update {
		require.NoError(t, os.RemoveAll(golden))
		for path, content := range got {
			writeProjectFile(t, filepath.Join(golden, filepath.FromSlash(path)), content)
		}
	}
	require.Equal(t, readProtos(t, golden), got)
	requireProtosCompile(t, projectDir, got)
}

// TestGenRunWritesNoProtobufDefinitionWhenAShapeCannotBeDescribed pins the
// diagnostics of the shapes protobuf cannot express, one per field, and that
// a failed run writes no pb/ file at all.
func TestGenRunWritesNoProtobufDefinitionWhenAShapeCannotBeDescribed(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/rejected.go": protobufRejectedModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	for _, want := range []string{
		"tmpapp/model.Rejected.untagged: the field has no pb tag; number it pb:\"N\" with N from 11",
		"tmpapp/model.Rejected.low: the pb tag names field number 3, but 1 to 10 belong to the framework's base fields; number business fields from 11",
		"tmpapp/model.Rejected.reserved: the pb tag names field number 19500, inside the range 19000 to 19999 protobuf reserves",
		"tmpapp/model.Rejected.twice: field number 11 is already taken by title; give each field its own number",
		"tmpapp/model.Rejected.matrix: a slice of slices or maps has no protobuf type; wrap the element in a struct type",
		"tmpapp/model.Rejected.speaker: an interface with methods has no protobuf type, the dynamic type decides it; use a concrete type",
		"tmpapp/model.Rejected.comment: type database/sql.NullString is declared outside the project, so its fields cannot carry pb tags; use a project type",
		"tmpapp/model.Rejected.word: the pb tag \"eleven\" is not a field number; write the number alone, as in pb:\"11\"",
	} {
		require.Contains(t, err.Error(), want)
	}
	_, statErr := os.Stat(filepath.Join(projectDir, "pb"))
	require.True(t, os.IsNotExist(statErr), "a failed run must write no file, stat error = %v", statErr)
}

// TestGenRunRefusesTwoActionsBecomingOneRPC pins the refusal of two actions
// of one model that would become one rpc: the same action on two routes that
// add no path parameter of their own.
func TestGenRunRefusesTwoActionsBecomingOneRPC(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/clash.go": protobufClashModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Clash: the List actions on routes clashes and public/clashes both become rpc ListClash; name one of them with Filename()")
}

// TestGenRunRefusesATypeNamedLikeAStandardMessage pins that a project type
// reached after a standard message took its name is reported with the rpc
// holding the name, instead of the file doubling the message.
func TestGenRunRefusesATypeNamedLikeAStandardMessage(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/notice.go": protobufStandardNameModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.CreateNoticeRequest: the message CreateNoticeRequest clashes with the rpc NoticeService.CreateNotice; rename the type")
}

// TestGenRunRefusesARouteParameterNamedLikeAField pins that a route parameter
// whose name a request message already uses for a field of its own is
// reported: the parameter would have no field to travel in.
func TestGenRunRefusesARouteParameterNamedLikeAField(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/entry.go": protobufParamClashModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Entry: the :page parameter of pages/:page/entries clashes with the page field of ListEntryByPageRequest; rename the parameter")
}

// writeProtobufProject writes the model files of a project whose models are
// served over gRPC. Single quotes in the sources stand for backquotes.
func writeProtobufProject(t *testing.T, projectDir string, files map[string]string) {
	t.Helper()

	for path, source := range files {
		writeProjectFile(t, filepath.Join(projectDir, filepath.FromSlash(path)), strings.ReplaceAll(source, "'", "`"))
	}
}

// readProtos reads every .proto file under root, keyed by its slash-separated
// path relative to root, record/item.proto for the file gg gen writes to
// pb/record/item.proto.
func readProtos(t *testing.T, root string) map[string]string {
	t.Helper()

	files := make(map[string]string)
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	}))
	return files
}

// requireProtosCompile compiles the generated files, named relative to pb/,
// with pb/ as the import root, the way protoc would with -I pb, so a file
// protoc would refuse fails the test.
func requireProtosCompile(t *testing.T, projectDir string, files map[string]string) {
	t.Helper()

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{filepath.Join(projectDir, "pb")},
		}),
	}
	names := make([]string, 0, len(files))
	for path := range files {
		names = append(names, path)
	}
	_, err := compiler.Compile(context.Background(), names...)
	require.NoError(t, err)
}

const protobufRecordModel = `package model

import (
	"encoding/json"
	"time"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a note kept by the record service.
type Record struct {
	// Title is the display title.
	Title   string            'json:"title" pb:"11"'
	Status  RecordStatus      'json:"status" pb:"12"'
	Summary *string           'json:"summary,omitempty" pb:"13"'
	Tags    []string          'json:"tags,omitempty" pb:"14" gorm:"-"'
	Labels  map[string]string 'json:"labels,omitempty" pb:"15" gorm:"-"'
	Count   int               'json:"count" pb:"16"'
	Ratio   float64           'json:"ratio" pb:"17"'
	Enabled bool              'json:"enabled" pb:"18"'
	Payload []byte            'json:"payload,omitempty" pb:"19"'
	Raw     json.RawMessage   'json:"raw,omitempty" pb:"20"'
	Extra   map[string]any    'json:"extra,omitempty" pb:"21" gorm:"-"'
	Due     time.Time         'json:"due" pb:"22"'
	Meta    RecordMeta        'json:"meta" pb:"23" gorm:"-"'
	Window  struct {
		From string 'json:"from" pb:"1"'
		To   string 'json:"to,omitempty" pb:"2"'
	} 'json:"window" pb:"24" gorm:"-"'
	Ignored string 'json:"-"'

	model.Base
}

// RecordStatus is the lifecycle state of a record.
type RecordStatus string

const (
	// RecordStatusActive marks a record in use.
	RecordStatusActive   RecordStatus = "active"
	RecordStatusArchived RecordStatus = "archived"
)

// RecordMeta is kept beside a record.
type RecordMeta struct {
	Author string 'json:"author" pb:"1"'
	Score  int32  'json:"score" pb:"2"'
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Param("record")
	dsl.Create(func() {})
	dsl.Delete(func() {})
	dsl.Update(func() {})
	dsl.Patch(func() {})
	dsl.List(func() {})
	dsl.Get(func() {})
	dsl.CreateMany(func() {})
	dsl.DeleteMany(func() {})
	dsl.UpdateMany(func() {})
	dsl.PatchMany(func() {})
	dsl.Route("/owners/:owner/records", func() {
		dsl.List(func() {})
	})
}
`

const protobufItemModel = `package record

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Item belongs to a record.
type Item struct {
	Content string 'json:"content" pb:"11"'
	Links   []Link 'json:"links,omitempty" pb:"12" gorm:"-"'

	model.Base
}

// Link points from an item to a page.
type Link struct {
	URL   string 'json:"url" pb:"1"'
	Title string 'json:"title,omitempty" pb:"2"'
}

// MergeReq asks to merge items into one.
type MergeReq struct {
	IDs []string 'json:"ids" pb:"1"'
}

// MergeRsp answers a merge with the item kept.
type MergeRsp struct {
	Item *Item 'json:"item" pb:"1"'
}

// MergedItemRsp is what the merge action answers with: the alias shares the
// message of MergeRsp.
type MergedItemRsp = MergeRsp

func (Item) TableName() string { return "items" }

func (Item) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("items")
	dsl.Create(func() {})
	dsl.Get(func() {})
	dsl.Route("items/merge", func() {
		dsl.Create(func() {
			dsl.Filename("merge")
			dsl.Service()
			dsl.Payload[*MergeReq]()
			dsl.Result[*MergedItemRsp]()
		})
	})
	dsl.Route("items/:id/seal", func() {
		dsl.Create(func() {
			dsl.Filename("seal")
		})
	})
}
`

const protobufReportModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Report answers summaries; no table backs it.
type Report struct {
	model.Empty
}

// ReportRsp is a summary.
type ReportRsp struct {
	Total int64 'json:"total" pb:"1"'
}

func (Report) Design() {
	dsl.GRPC()
	dsl.Route("/reports/summary", func() {
		dsl.Get(func() {
			dsl.Exact()
			dsl.Service()
			dsl.Result[*ReportRsp]()
		})
	})
}
`

// protobufNoteModel is the model of the pb.Generate doc comment; the golden
// file note.proto is the example that comment shows.
const protobufNoteModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Note is a note kept by the note service.
type Note struct {
	// Title is the display title.
	Title string   'json:"title" pb:"11"'
	Tags  []string 'json:"tags,omitempty" pb:"12" gorm:"-"'

	model.Base
}

func (Note) TableName() string { return "notes" }

func (Note) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("notes")
	dsl.Create(func() {})
	dsl.Get(func() {})
}
`

const protobufPlainModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Plain is served over HTTP only.
type Plain struct {
	Name string 'json:"name"'

	model.Base
}

func (Plain) TableName() string { return "plains" }

func (Plain) Design() {
	dsl.Migrate()
	dsl.Endpoint("plains")
	dsl.Create(func() {})
}
`

const protobufRejectedModel = `package model

import (
	"database/sql"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Rejected carries a field of every shape protobuf cannot express.
type Rejected struct {
	Title    string         'json:"title" pb:"11"'
	Untagged string         'json:"untagged"'
	Low      string         'json:"low" pb:"3"'
	Reserved string         'json:"reserved" pb:"19500"'
	Twice    string         'json:"twice" pb:"11"'
	Matrix   [][]string     'json:"matrix" pb:"12" gorm:"-"'
	Voice    Speaker        'json:"speaker" pb:"13" gorm:"-"'
	Comment  sql.NullString 'json:"comment" pb:"14"'
	Word     string         'json:"word" pb:"eleven"'

	model.Base
}

// Speaker is an interface with methods.
type Speaker interface {
	Speak() string
}

func (Rejected) TableName() string { return "rejected" }

func (Rejected) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("rejected")
	dsl.Create(func() {})
}
`

// protobufStandardNameModel reaches a project type named like the request
// message of a standard action after that message took the name.
const protobufStandardNameModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Notice is created over gRPC.
type Notice struct {
	Title string 'json:"title" pb:"11"'

	model.Base
}

func (Notice) TableName() string { return "notices" }

func (Notice) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("notices")
	dsl.Create(func() {})
	dsl.Route("notices/echo", func() {
		dsl.Create(func() {
			dsl.Filename("echo")
			dsl.Payload[*EchoReq]()
			dsl.Result[*EchoRsp]()
		})
	})
}

// EchoReq carries the draft to echo.
type EchoReq struct {
	Draft CreateNoticeRequest 'json:"draft" pb:"1"'
}

// EchoRsp answers with the draft.
type EchoRsp struct {
	Draft CreateNoticeRequest 'json:"draft" pb:"1"'
}

// CreateNoticeRequest is a project type named like the request message of
// NoticeService.Create.
type CreateNoticeRequest struct {
	Title string 'json:"title" pb:"1"'
}
`

// protobufParamClashModel lists entries under a route whose parameter is
// named like a field of every List request.
const protobufParamClashModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Entry is listed by page.
type Entry struct {
	Title string 'json:"title" pb:"11"'

	model.Base
}

func (Entry) TableName() string { return "entries" }

func (Entry) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("entries")
	dsl.Route("/pages/:page/entries", func() {
		dsl.List(func() {})
	})
}
`

const protobufClashModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Clash lists on two routes that add no parameter.
type Clash struct {
	Name string 'json:"name" pb:"11"'

	model.Base
}

func (Clash) TableName() string { return "clashes" }

func (Clash) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("clashes")
	dsl.List(func() {})
	dsl.Route("/public/clashes", func() {
		dsl.List(func() {
			dsl.Public()
		})
	})
}
`
