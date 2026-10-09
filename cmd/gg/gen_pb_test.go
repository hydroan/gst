package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// TestGenRunWritesTheProtobufDefinitionsOfGRPCModels holds the .proto files
// gg gen writes for the models declaring GRPC(), and the Go files it writes
// beside them, against testdata/pb/golden; run it with -update to rewrite
// them. There is one .proto per model file under pb/, mirroring the model
// directory, with the model's message, the messages of its standard
// actions, the Go types of its custom actions and its service; beside it a
// .gen.go with the type serving the service, the calls of its actions, the
// handlers of its rpcs and the conversions of its messages; and, in every
// package under pb/, a pb.gen.go registering its services, the root one
// importing the packages below it. A model without GRPC() gets no file. A model
// referring to a type of another directory, Pin's Link of
// model/record/item.go, gets a definition importing the other's by its
// registered path, tmpapp/record/item.proto, and conversions calling the
// other package's. The definitions are compiled the way protoc compiles
// them as well. The files
// are where the examples in the doc comments of the pb package come from:
// pb.Generate's whole note.proto, the excerpts of buildMessage,
// declareService, rpcMessages, customRequest, customResponse, queryFields
// and descriptor; the whole report.gen.go of handlerFile, the excerpts of
// serviceType, actionCalls, handler, streamHandler, toProto, fromProto and
// conversionFuncs; and the two pb.gen.go of registrationFiles.
// TestPBDocExamplesComeFromTheGoldenFiles holds the two together: every code
// block of those doc comments must be a passage of a file here, or of the
// model sources this test writes.
//
// Beside every .proto the run writes the Go files the protobuf plugins
// compile from it (see pb.Compile): the messages in note.pb.go and the
// service in note_grpc.pb.go, and the project builds with them; main.go
// imports the pb package for the registration.
func TestGenRunWritesTheProtobufDefinitionsOfGRPCModels(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{
		"model/record.go":      protobufRecordModel,
		"model/record/item.go": protobufItemModel,
		"model/report.go":      protobufReportModel,
		"model/plain.go":       protobufPlainModel,
		"model/note.go":        protobufNoteModel,
		"model/pin.go":         protobufPinModel,
		"model/shape.go":       protobufShapeModel,
		"model/feed.go":        protobufFeedModel,
	})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	got := readGenerated(t, filepath.Join(projectDir, "pb"))
	golden := filepath.Join(frameworkRepoRoot(t), "cmd", "gg", "testdata", "pb", "golden")
	if *update {
		require.NoError(t, os.RemoveAll(golden))
		for path, content := range got {
			writeProjectFile(t, filepath.Join(golden, filepath.FromSlash(path)), content)
		}
	}
	require.Equal(t, readGenerated(t, golden), got)
	// The definitions are registered under the module path, which the Go
	// files compiled from them name.
	service, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, "feed_grpc.pb.go"))
	require.NoError(t, err)
	require.Contains(t, string(service), `Metadata: "tmpapp/feed.proto"`)
	protos := make(map[string]string)
	for path, content := range got {
		if strings.HasSuffix(path, ".proto") {
			protos[path] = content
		}
	}
	requireProtosCompile(t, protos)

	for path := range protos {
		base := strings.TrimSuffix(path, ".proto")
		require.FileExists(t, filepath.Join(projectDir, "pb", filepath.FromSlash(base+".pb.go")))
		require.FileExists(t, filepath.Join(projectDir, "pb", filepath.FromSlash(base+"_grpc.pb.go")), "every model file declares a service")
		require.Contains(t, got, base+ggconst.SuffixGenGo, "every definition gets its handlers file")
	}
	require.Contains(t, got, ggconst.FilePBGen)
	mainCode, err := os.ReadFile(filepath.Join(projectDir, ggconst.FileMain))
	require.NoError(t, err)
	require.Contains(t, string(mainCode), `_ "tmpapp/pb"`)
	build := exec.Command("go", "build", "./pb/...")
	build.Dir = projectDir
	output, err := build.CombinedOutput()
	require.NoError(t, err, "the generated Go files must build: %s", output)

	// The conversions run for real: a value of every kind of field goes
	// through its message and comes back as it went in.
	writeProtobufProject(t, projectDir, map[string]string{"pb/convert_test.go": protobufConversionTest})
	test := exec.Command("go", "test", "-trimpath", "./pb/")
	test.Dir = projectDir
	output, err = test.CombinedOutput()
	require.NoError(t, err, "the generated conversions must round-trip every value: %s", output)
}

// TestGenRunTypeChecksTheGeneratedGoFiles pins the check gg gen runs on the
// Go files it is about to write under pb/ (see pb.TypeCheck): the files of
// a project type-check as the packages they make up; a file the compiler
// would refuse is reported at its line, under its package; and a project
// whose imports cannot be listed is reported as unchecked, which gg gen
// warns about and writes the files all the same.
func TestGenRunTypeChecksTheGeneratedGoFiles(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	var files []pb.File
	require.NoError(t, filepath.WalkDir(filepath.Join(projectDir, ggconst.DirPB), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ggconst.ExtensionGo) {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, pb.File{Path: filepath.ToSlash(rel), Content: string(content)})
		return nil
	}))
	require.NotEmpty(t, files)
	require.NoError(t, pb.TypeCheck(projectDir, "tmpapp", files))

	broken := append(slices.Clone(files), pb.File{Path: "pb/broken.gen.go", Content: "package pb\n\nvar broken = missing\n"})
	err := pb.TypeCheck(projectDir, "tmpapp", broken)
	var diagnostics *pb.DiagnosticsError
	require.ErrorAs(t, err, &diagnostics)
	require.Contains(t, err.Error(), "pb/broken.gen.go:3: tmpapp/pb: undefined: missing")

	unresolved := t.TempDir()
	writeProjectFile(t, filepath.Join(unresolved, "go.mod"), "module unresolved\n\ngo "+strings.TrimPrefix(runtime.Version(), "go")+"\n")
	err = pb.TypeCheck(unresolved, "unresolved", []pb.File{{Path: "pb/note.gen.go", Content: "package pb\n\nimport \"github.com/hydroan/gst/grpc\"\n\nvar _ = grpc.MethodStream\n"}})
	require.ErrorIs(t, err, pb.ErrUnchecked)
}

// TestGenRunServesStreamActionsOverGRPCAlone pins what gg gen makes of a
// model whose actions are Stream actions alone: its rpcs are derived, each
// streaming the side it declares and described by the route it is declared
// on rather than a path; its services register and get their files with
// the Stream method of their kind and a gRPC test scaffold, but no route
// and no router registration, so the project builds with a router file
// naming it nowhere.
func TestGenRunServesStreamActionsOverGRPCAlone(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/feed.go": protobufFeedModel})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	proto, err := os.ReadFile(filepath.Join(projectDir, "pb", "feed.proto"))
	require.NoError(t, err)
	require.Contains(t, string(proto), "// WatchFeed is the Stream action of Feed declared on feeds/watch, served over gRPC alone.")
	require.Contains(t, string(proto), "rpc WatchFeed ( WatchFeedRequest ) returns ( stream WatchFeedResponse );")
	require.Contains(t, string(proto), "rpc UploadFeedByFeed ( stream UploadFeedByFeedRequest ) returns ( UploadFeedByFeedResponse );")
	require.Contains(t, string(proto), "rpc ChatFeed ( stream ChatFeedRequest ) returns ( stream ChatFeedResponse );")
	require.Contains(t, string(proto), "rpc IngestFeed ( stream IngestFeedRequest ) returns ( IngestFeedResponse );", "a client stream declaring no Result answers an empty message")
	router, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirRouter, ggconst.FileRouterGen))
	require.NoError(t, err)
	require.NotContains(t, string(router), "feeds", "the router registers nothing for a Stream")
	services, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirService, ggconst.FileServiceGen))
	require.NoError(t, err)
	require.Contains(t, string(services), `service.Register[*feed.Watch](consts.Stream, "/api/feeds/watch")`)
	require.Contains(t, string(services), `service.Register[*feed.Upload](consts.Stream, "/api/feeds/:feed/upload")`)
	for name, signature := range map[string]string{
		"watch":  "func (w *Watch) Stream(ctx *gst.ServiceContext, req *model.FeedWatchReq, stream *grpc.ServerStream[*model.FeedWatchRsp]) (err error)",
		"tail":   "func (t *Tail) Stream(ctx *gst.ServiceContext, req *gstmodel.Empty, stream *grpc.ServerStream[*model.FeedTailRsp]) (err error)",
		"upload": "func (u *Upload) Stream(ctx *gst.ServiceContext, stream *grpc.ClientStream[*model.FeedUploadReq]) (rsp *model.FeedUploadRsp, err error)",
		"chat":   "func (c *Chat) Stream(ctx *gst.ServiceContext, stream *grpc.BidiStream[*model.FeedChatReq, *model.FeedChatRsp]) (err error)",
	} {
		code, readErr := os.ReadFile(filepath.Join(projectDir, ggconst.DirService, "feed", name+".go"))
		require.NoError(t, readErr, name)
		require.Contains(t, string(code), signature, name)
	}
	scaffold, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirService, "feed", "upload_test.go"))
	require.NoError(t, err)
	require.Contains(t, string(scaffold), "// TestUpload covers the UploadFeedByFeed rpc, served by Upload in upload.go.")
	require.Contains(t, string(scaffold), "client := pb.NewFeedServiceClient(conn)")
	require.Contains(t, string(scaffold), "rsp, err := stream.CloseAndRecv()")
	mainTest, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirService, "feed", ggconst.FileMainTest))
	require.NoError(t, err)
	require.Contains(t, string(mainTest), `_ "tmpapp/pb"`, "TestMain registers the gRPC services the way main.go does")
	// go vet compiles the test files, the scaffolds among them.
	requireProjectCompiles(t)
}

// TestGenRunImportsThePBPackageWhileServingGRPC pins that main.go imports
// the pb package, for the init function registering the services, exactly
// while a model declares GRPC(): once the last declaration is gone the
// import goes with it, whether or not the stale files under pb/ were
// pruned.
func TestGenRunImportsThePBPackageWhileServingGRPC(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	mainCode, err := os.ReadFile(filepath.Join(projectDir, ggconst.FileMain))
	require.NoError(t, err)
	require.Contains(t, string(mainCode), `_ "tmpapp/pb"`)

	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "\tdsl.GRPC()\n", "", 1)})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	mainCode, err = os.ReadFile(filepath.Join(projectDir, ggconst.FileMain))
	require.NoError(t, err)
	require.NotContains(t, string(mainCode), "tmpapp/pb", "without a model served over gRPC main.go must not import the package")
	require.NoFileExists(t, filepath.Join(projectDir, ggconst.DirPB, ggconst.FilePBGen), "the Go files go with the last model served over gRPC")
	require.FileExists(t, filepath.Join(projectDir, ggconst.DirPB, "note.proto"), "the definition stays, with what it reserves, until pruned")
}

// TestGenRunRemovesTheGoFilesOfAModelNoLongerServedOverGRPC pins that gg gen
// deletes the Go files under pb/ it did not write this run, the handlers
// and the plugins' files of a model deleted or no longer declaring GRPC(),
// so that the project builds without a prune, while the definition stays,
// with the numbers it reserves, for prune to delete; a file a gst.yaml
// prune.ignore entry covers stays as well.
func TestGenRunRemovesTheGoFilesOfAModelNoLongerServedOverGRPC(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel, "model/feed.go": protobufFeedModel})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	derived := []string{"feed.gen.go", "feed.pb.go", "feed_grpc.pb.go"}
	for _, name := range derived {
		require.FileExists(t, filepath.Join(projectDir, ggconst.DirPB, name))
	}

	require.NoError(t, os.Remove(filepath.Join(projectDir, "model", "feed.go")))
	require.NoError(t, os.RemoveAll(filepath.Join(projectDir, ggconst.DirService, "feed")))
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	for _, name := range derived {
		require.NoFileExists(t, filepath.Join(projectDir, ggconst.DirPB, name))
	}
	require.FileExists(t, filepath.Join(projectDir, ggconst.DirPB, "feed.proto"))
	registration, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, ggconst.FilePBGen))
	require.NoError(t, err)
	require.NotContains(t, string(registration), "FeedService")
	build := exec.Command("go", "build", "./pb/...")
	build.Dir = projectDir
	output, err := build.CombinedOutput()
	require.NoError(t, err, "the project builds without the stale files: %s", output)

	t.Run("a file prune.ignore covers stays", func(t *testing.T) {
		writeProtobufProject(t, projectDir, map[string]string{"model/feed.go": protobufFeedModel})
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		writeProjectFile(t, filepath.Join(projectDir, "gst.yaml"), "prune:\n  ignore:\n    - pb/feed.gen.go\n")
		require.NoError(t, os.Remove(filepath.Join(projectDir, "model", "feed.go")))
		require.NoError(t, os.RemoveAll(filepath.Join(projectDir, ggconst.DirService, "feed")))

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		require.FileExists(t, filepath.Join(projectDir, ggconst.DirPB, "feed.gen.go"))
		require.NoFileExists(t, filepath.Join(projectDir, ggconst.DirPB, "feed.pb.go"))
	})
}

// TestGenRunCommentsTheFrameworkBaseFields pins the comment each key of the
// framework's model base carries in every model message: the base is
// declared outside the project, where the generator reads no doc comment,
// so it writes the fixed one of each key, which is what buf's COMMENT_FIELD
// rule asks of every field. Guards buildMessage's example.
func TestGenRunCommentsTheFrameworkBaseFields(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	got := readGenerated(t, filepath.Join(projectDir, "pb"))
	require.Contains(t, got["note.proto"], `message Note {
  // The identifier of the record, assigned when it is created.
  string id = 1;

  // The id of the user who created the record.
  string created_by = 2;

  // The id of the user who last updated the record.
  string updated_by = 3;

  // When the record was created.
  google.protobuf.Timestamp created_at = 4;

  // When the record was last updated.
  google.protobuf.Timestamp updated_at = 5;
`)
}

// TestGenRunRefusesAModelFileNamedPB pins that a model file named pb.go is
// reported: its handlers file would be pb/pb.gen.go, the registration file.
func TestGenRunRefusesAModelFileNamedPB(t *testing.T) {
	for _, tt := range []struct {
		name, file, want string
	}{
		{"in the model directory", "model/pb.go", "pb/pb.proto: the model file model/pb.go would get its handlers at pb/pb.gen.go, the registration file; rename the file"},
		{"in a directory below it", "model/record/pb.go", "pb/record/pb.proto: the model file model/record/pb.go would get its handlers at pb/record/pb.gen.go, the registration file; rename the file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			projectDir, ok := newGenProject(t)
			if !ok {
				return
			}
			source := protobufPBFileModel
			if tt.file == "model/record/pb.go" {
				source = strings.Replace(source, "package model", "package record", 1)
			}
			writeProtobufProject(t, projectDir, map[string]string{tt.file: source})

			err := genRunWithOptions(genRunOptions{Quiet: true})

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestGenRunRegistersTheServicesOfEachPackageInItsOwnFile pins where the
// registrations go: every package under pb/ registers its own services in
// its pb.gen.go, through the unexported type serving each, which nothing
// outside the package can register elsewhere; and the root pb/pb.gen.go,
// the one main.go imports, imports the packages below it, so that a project
// whose services all live below the root still registers them through the
// one import.
func TestGenRunRegistersTheServicesOfEachPackageInItsOwnFile(t *testing.T) {
	read := func(t *testing.T, projectDir string, path string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(path)))
		require.NoError(t, err)
		return string(content)
	}

	t.Run("a package below the root beside services at the root", func(t *testing.T) {
		projectDir, ok := newGenProject(t)
		if !ok {
			return
		}
		writeProtobufProject(t, projectDir, map[string]string{
			"model/record.go":      protobufRecordModel,
			"model/record/item.go": protobufItemModel,
		})

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		root := read(t, projectDir, "pb/pb.gen.go")
		require.Contains(t, root, "grpc.Register[RecordServiceServer](RegisterRecordServiceServer, recordService{},")
		require.Contains(t, root, `_ "tmpapp/pb/record"`)
		require.NotContains(t, root, "ItemService", "a service below the root registers in its own package")
		below := read(t, projectDir, "pb/record/pb.gen.go")
		require.Contains(t, below, "package record")
		require.Contains(t, below, "grpc.Register[ItemServiceServer](RegisterItemServiceServer, itemService{},")
		handlers := read(t, projectDir, "pb/record/item.gen.go")
		require.Contains(t, handlers, "type itemService struct")
		require.Contains(t, handlers, "func (itemService) CreateItem(")
	})
	t.Run("every service below the root", func(t *testing.T) {
		projectDir, ok := newGenProject(t)
		if !ok {
			return
		}
		writeProtobufProject(t, projectDir, map[string]string{"model/record/item.go": protobufItemModel})

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		root := read(t, projectDir, "pb/pb.gen.go")
		require.Contains(t, root, `_ "tmpapp/pb/record"`)
		require.NotContains(t, root, "grpc.Register", "the root has no service of its own to register")
		require.Contains(t, read(t, projectDir, "pb/record/pb.gen.go"), "grpc.Register[ItemServiceServer](RegisterItemServiceServer, itemService{},")
		require.Contains(t, read(t, projectDir, ggconst.FileMain), `_ "tmpapp/pb"`)
		build := exec.Command("go", "build", "./...")
		build.Dir = projectDir
		output, err := build.CombinedOutput()
		require.NoError(t, err, "the project must build: %s", output)
	})
}

// TestGenRunListsTheQueryFieldsAModelReads pins that the List request of a
// model's standard action carries the query fields the model reads and no
// other: the filters always, the page and size for a model embedding
// model.Pagination, the size and the cursor for one embedding model.Cursor,
// and the orderings and expansion, with all of those, for one embedding
// model.Query; the handler reads the fields present. The Get request carries
// the expansion whatever the model embeds, the get flow reading it for
// every model.
func TestGenRunListsTheQueryFieldsAModelReads(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{
		"model/plain.go":    protobufQueryModel("Plain", ""),
		"model/paged.go":    protobufQueryModel("Paged", "model.Pagination"),
		"model/cursored.go": protobufQueryModel("Cursored", "model.Cursor"),
		"model/queried.go":  protobufQueryModel("Queried", "model.Query"),
	})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	for _, tt := range []struct {
		model string
		list  []string
	}{
		{"Plain", []string{"filters"}},
		{"Paged", []string{"filters", "page", "size"}},
		{"Cursored", []string{"filters", "size", "cursor_field", "cursor_value", "cursor_next"}},
		{"Queried", []string{"filters", "sort_by", "page", "size", "cursor_field", "cursor_value", "cursor_next", "expand", "depth"}},
	} {
		content, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, strings.ToLower(tt.model)+".proto"))
		require.NoError(t, err)
		require.Equal(t, tt.list, messageFieldNames(string(content), "List"+tt.model+"Request"), "the List request of %s", tt.model)
		require.Equal(t, []string{"id", "expand", "depth"}, messageFieldNames(string(content), "Get"+tt.model+"Request"), "the Get request of %s", tt.model)
	}
	build := exec.Command("go", "build", "./pb/...")
	build.Dir = projectDir
	output, err := build.CombinedOutput()
	require.NoError(t, err, "the handlers read the fields present: %s", output)
}

// messageFieldNames returns the names of the fields the top-level message
// named name declares in the definition proto, in order, the fields of a
// message nested in it left out.
func messageFieldNames(proto, name string) []string {
	_, rest, found := strings.Cut(proto, "message "+name+" {\n")
	if !found {
		return nil
	}
	block, _, _ := strings.Cut(rest, "\n}\n")
	var names []string
	for line := range strings.SplitSeq(block, "\n") {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") || strings.HasPrefix(line, "  //") || !strings.HasSuffix(line, ";") {
			continue
		}
		words := strings.Fields(line)
		names = append(names, words[len(words)-3])
	}
	return names
}

// protobufQueryModel is a model declaring List and Get and embedding embed,
// a query marker of the framework, or nothing when embed is empty: what
// TestGenRunListsTheQueryFieldsAModelReads reads the request messages of.
func protobufQueryModel(name, embed string) string {
	if embed != "" {
		embed = "\t" + embed + "\n"
	}
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ` + name + ` struct {
	Title string 'json:"title" pb:"11"'

` + embed + `	model.Base
}

func (` + name + `) TableName() string { return "` + strings.ToLower(name) + `s" }

func (` + name + `) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("` + strings.ToLower(name) + `s")
	dsl.List(func() {})
	dsl.Get(func() {})
}
`
}

// TestGenRunRefusesAModelFileNamedLikeAPluginOutput pins that a model file
// ending in _grpc is refused: the protobuf plugin writes the service of
// record.proto to record_grpc.pb.go, where the messages of record_grpc.proto
// would go too.
func TestGenRunRefusesAModelFileNamedLikeAPluginOutput(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/record_grpc.go": protobufPBFileModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "pb/record_grpc.proto: the model file model/record_grpc.go ends in _grpc, the suffix of the service file the protobuf plugin writes for model/record.go; rename the file")
}

// TestGenRunRefusesNumberingFieldsSharingADeclaration pins that fields
// declared together, X, Y int32, are not numbered: one tag would number
// both alike, so the run stops with the file as it was, naming them.
func TestGenRunRefusesNumberingFieldsSharingADeclaration(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/spot.go": protobufSharedDeclarationModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "model/spot.go:10: X, Y share one declaration, which one pb tag would number alike; declare each field on a line of its own, then run gg gen again")
	kept, readErr := os.ReadFile(filepath.Join(projectDir, "model", "spot.go"))
	require.NoError(t, readErr)
	require.NotContains(t, string(kept), "pb:\"")
}

// TestGenRunRefusesNumberingAFieldTheMessagesDisagreeOn pins that a field of
// a struct embedded in two models, which the two messages would number
// differently, is not numbered: the run stops naming both numbers, since
// the one a message leaves free may be taken in the other.
func TestGenRunRefusesNumberingAFieldTheMessagesDisagreeOn(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{
		"model/audit.go":  protobufSharedEmbeddedAudit,
		"model/note.go":   protobufSharedEmbeddedNote,
		"model/record.go": protobufSharedEmbeddedRecord,
	})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "model/audit.go:5: Reviewer is embedded in messages that would number it ")
	require.Contains(t, err.Error(), "; number it by hand with a pb tag each of them leaves free")
	kept, readErr := os.ReadFile(filepath.Join(projectDir, "model", "audit.go"))
	require.NoError(t, readErr)
	require.NotContains(t, string(kept), "pb:\"")
}

// TestGenRunNamesTheFilesImportingEachOther pins the report of two model
// files whose types refer to each other, which the definitions cannot
// import in a cycle: both files are named, with the way out.
func TestGenRunNamesTheFilesImportingEachOther(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{
		"model/author.go": protobufCycleAuthorModel,
		"model/book.go":   protobufCycleBookModel,
	})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "the generated files author.proto and book.proto import each other in a cycle: a type of one Go file refers to a type of the other and back; keep the types referring to each other in one Go file")
}

// TestGenRunWritesNoProtobufDefinitionWhenAShapeCannotBeDescribed pins the
// diagnostics of the shapes protobuf cannot express, one per field, that a
// field without a pb tag is numbered instead of reported (see
// TestGenRunNumbersTheFieldsWithoutPBTags) while a field whose tag cannot be
// read is reported and left as written, a second run changing nothing, and
// that a failed run writes no file at all, the registration files included:
// the run stops on the diagnostics before anything is written.
func TestGenRunWritesNoProtobufDefinitionWhenAShapeCannotBeDescribed(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/rejected.go": protobufRejectedModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	// The one field without a tag is numbered rather than reported: after
	// the numbers the tagged fields hold.
	healed, readErr := os.ReadFile(filepath.Join(projectDir, "model", "rejected.go"))
	require.NoError(t, readErr)
	require.Regexp(t, "Untagged string +`json:\"untagged\" pb:\"18\"`", string(healed))
	require.NotContains(t, err.Error(), "Rejected.untagged")
	// The tag that cannot be read is left as it is: numbering it would add a
	// pb tag beside the unreadable one on every run.
	require.Regexp(t, "Spaced +string +`json:\"spaced\" pb: \"21\"`", string(healed))
	for _, want := range []string{
		"tmpapp/model.Rejected.low: the pb tag names field number 3, but 1 to 10 belong to the framework's base fields; number business fields from 11",
		"tmpapp/model.Rejected.reserved: the pb tag names field number 19500, inside the range 19000 to 19999 protobuf reserves",
		"tmpapp/model.Rejected.twice: field number 11 is already taken by title; give each field its own number",
		"tmpapp/model.Rejected.matrix: a slice of slices or maps has no protobuf type; wrap the element in a struct type",
		"tmpapp/model.Rejected.speaker: an interface with methods has no protobuf type, the dynamic type decides it; use a concrete type",
		"tmpapp/model.Rejected.comment: type database/sql.NullString is declared outside the project, so its fields cannot carry pb tags; use a project type",
		"tmpapp/model.Rejected.word: the pb tag \"eleven\" is not a field number; write the number alone, as in pb:\"11\"",
		`tmpapp/model.Rejected.spaced: the struct tag cannot be read from "pb: \"21\"" on, so its pb tag is not found; write each pair as key:"value", as in pb:"11"`,
		"tmpapp/model.Rejected.note: the field is promoted through an embedded pointer, which a message has no way to leave unset; embed the struct by value",
		"tmpapp/model.Rejected.limits: a map of pointers to a scalar has no protobuf type, a value is never unset; use a map of values",
		"tmpapp/model.Rejected.slots: a slice of pointers to a scalar has no protobuf type, an element is never unset; use a slice of values",
		"tmpapp/model.Rejected.keyed: map key type tmpapp/model.RejectedKey declares MarshalText, which encoding/json and the JSON v2 experiment apply to keys differently; use a string or integer key type without it",
	} {
		require.Contains(t, err.Error(), want)
	}
	for _, path := range []string{"pb", filepath.Join("model", "model.gen.go"), filepath.Join("service", "service.gen.go")} {
		_, statErr := os.Stat(filepath.Join(projectDir, path))
		require.True(t, os.IsNotExist(statErr), "a failed run must write no file, %s: stat error = %v", path, statErr)
	}

	require.Error(t, genRunWithOptions(genRunOptions{Quiet: true}))
	again, readErr := os.ReadFile(filepath.Join(projectDir, "model", "rejected.go"))
	require.NoError(t, readErr)
	require.Equal(t, string(healed), string(again), "a second run leaves the model file as the first left it")
}

// TestGenRunNumbersTheFieldsWithoutPBTags pins that gg gen writes the pb
// tag of every field of a gRPC model that has none, with the number the
// generator chose: the next number after every one the tagged fields hold,
// starting past 10 in a model embedding the base and past 0 in any other
// struct, the nested and the referenced ones included, the file keeping its
// layout and comments (it holds the example of the rewritePBTags doc
// comment); and that a second run leaves the file as it is.
func TestGenRunNumbersTheFieldsWithoutPBTags(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/draft.go": protobufUntaggedModel})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	healed, err := os.ReadFile(filepath.Join(projectDir, "model", "draft.go"))
	require.NoError(t, err)
	require.Contains(t, string(healed), strings.ReplaceAll(`type Draft struct {
	Title  string   'json:"title" pb:"11"'
	Body   string   'json:"body" pb:"12"'
	Tags   []string 'json:"tags,omitempty" gorm:"-" pb:"13"' // trailing comment
	Window struct {
		From string 'json:"from" pb:"1"'
		To   string 'pb:"2"'
	} 'json:"window" gorm:"-" pb:"14"'
	Meta DraftMeta 'json:"meta" gorm:"-" pb:"15"'

	model.Base
}`, "'", "`"))
	// A struct without the base numbers from 1, after the highest number
	// its tagged fields hold: no hole below it is filled, a hole being a
	// number that may have been used.
	require.Contains(t, string(healed), strings.ReplaceAll(`type DraftMeta struct {
	Author string 'json:"author" pb:"6"'
	Score  int32  'json:"score" pb:"5"'
	Note   string 'json:"note" pb:"7"'
}`, "'", "`"))
	proto, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, "draft.proto"))
	require.NoError(t, err)
	require.Contains(t, string(proto), "string body = 12;")
	require.Contains(t, string(proto), "string note = 7;")

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	again, err := os.ReadFile(filepath.Join(projectDir, "model", "draft.go"))
	require.NoError(t, err)
	require.Equal(t, string(healed), string(again))
}

// TestGenRunNumbersTheFieldsWithoutPBTagsAfterTheCommittedDefinition pins
// the numbers gg gen gives against the definition already under pb/: a
// field the committed definition holds gets its number back, and a new
// field never takes a number the committed definition reserves, the one of
// a dropped field included.
func TestGenRunNumbersTheFieldsWithoutPBTagsAfterTheCommittedDefinition(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	t.Run("a committed field gets its number back", func(t *testing.T) {
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, ` pb:"12"`, "", 1)})

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		healed, err := os.ReadFile(filepath.Join(projectDir, "model", "note.go"))
		require.NoError(t, err)
		require.Contains(t, string(healed), "`json:\"tags,omitempty\" gorm:\"-\" pb:\"12\"`")
	})
	t.Run("a dropped field's number stays reserved", func(t *testing.T) {
		withoutTags := strings.Replace(protobufNoteModel, "\tTags  []string 'json:\"tags,omitempty\" pb:\"12\" gorm:\"-\"'\n", "\tBody string 'json:\"body\"'\n", 1)
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": withoutTags})

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		healed, err := os.ReadFile(filepath.Join(projectDir, "model", "note.go"))
		require.NoError(t, err)
		require.Contains(t, string(healed), "`json:\"body\" pb:\"13\"`")
		proto, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, "note.proto"))
		require.NoError(t, err)
		require.Contains(t, string(proto), "reserved 12;")
	})
}

// TestGenRunRefusesTwoActionsBecomingOneRPC pins the refusal of two actions
// of one model that would become one rpc: the same action on two routes that
// add no path parameter of their own.
func TestGenRunRefusesTwoActionsBecomingOneRPC(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/clash.go": protobufClashModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Clash: the List actions on routes clashes and public/clashes both become rpc ListClash; name one of them with Service(\"name\")")
}

// TestGenRunRefusesATypeNamedLikeAStandardMessage pins that a project type
// reached after a standard message took its name is reported with the rpc
// holding the name, instead of the file doubling the message.
func TestGenRunRefusesATypeNamedLikeAStandardMessage(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/notice.go": protobufStandardNameModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.CreateNoticeRequest: the message CreateNoticeRequest clashes with the rpc NoticeService.CreateNotice; rename the type")
}

// TestGenRunRefusesAnUnnamedStructNamedLikeAType pins that the message of an
// unnamed struct field, named after the enclosing message and the field,
// is reported when a type of the file already takes that name: two messages
// cannot share it.
func TestGenRunRefusesAnUnnamedStructNamedLikeAType(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/box.go": protobufUnnamedClashModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Box.lid: the unnamed struct of the field becomes the message BoxLid, which clashes with the type tmpapp/model.BoxLid; name the field or the type differently")
}

// TestGenRunRefusesARouteParameterNamedLikeAField pins that a route parameter
// whose name a request message already uses for a field of its own is
// reported: the parameter would have no field to travel in.
func TestGenRunRefusesARouteParameterNamedLikeAField(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/entry.go": protobufParamClashModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Entry: the :page parameter of /api/pages/:page/entries clashes with the page field of ListEntryByPageRequest; rename the parameter")
}

// TestGenRunRefusesARouteParameterNamedLikeAQueryField pins that a route
// parameter named like a query field of a List request is reported even
// when the model reads no such control and the request carries no such
// field: the name is the query's in every List request, which is where the
// handler would read it from.
func TestGenRunRefusesARouteParameterNamedLikeAQueryField(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/entry.go": strings.Replace(protobufParamClashModel, "\tmodel.Pagination\n", "", 1)})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Entry: the :page parameter of /api/pages/:page/entries takes the name of the page query field of a List request; rename the parameter")
}

// TestGenRunRefusesAFieldReachedThroughAnUnexportedEmbeddedStruct pins that
// a field the generated conversions can only select through an unexported
// embedded struct, its name shadowed by a field declared nearer the
// surface, is reported: the embedded field's name is one no other package
// can write, so the field is reported with the ways out.
func TestGenRunRefusesAFieldReachedThroughAnUnexportedEmbeddedStruct(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/entry.go": protobufUnexportedEmbeddingModel})

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Entry.name: the field is selected through the unexported embedded field entryMeta, which the generated code cannot name; export entryMeta, or declare the field on Entry")
}

// TestGenRunAliasesAnImportNamedLikeAGeneratedLocal pins that a project
// package named like a local of the generated code, data for the value a
// JSON wrapper is decoded into, is imported under an alias: a package
// imported under its own name would be shadowed where the local is in
// scope.
func TestGenRunAliasesAnImportNamedLikeAGeneratedLocal(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/data/sample.go": protobufDataPackageModel})

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	handlers, err := os.ReadFile(filepath.Join(projectDir, ggconst.DirPB, "data", "sample.gen.go"))
	require.NoError(t, err)
	require.Contains(t, string(handlers), `model_data "tmpapp/model/data"`)
	require.Contains(t, string(handlers), "var data model_data.SampleOptions")
}

// TestGenRunRefusesAGRPCModelWithNothingToServe pins that a model declaring
// GRPC() none of whose actions gRPC serves, every one ignored by gst.yaml or
// HTTP only, is reported instead of getting a service without an rpc.
func TestGenRunRefusesAGRPCModelWithNothingToServe(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/silent.go": protobufIgnoredModel})
	writeProjectFile(t, filepath.Join(projectDir, "gst.yaml"), "gen:\n  routes:\n    ignore:\n      /api/silents: [GET]\n")

	err := genRunWithOptions(genRunOptions{Quiet: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tmpapp/model.Silent: the model declares GRPC() but none of its actions is served over gRPC, every one being ignored by gst.yaml or HTTP only; remove GRPC() or declare an action gRPC serves")
}

// TestGenRunHoldsTheCommittedDefinitionsToTheirNumbers pins that gg gen
// reads the .proto already on disk as the contract in force: a field
// changing number, or a number changing hands, is refused with the file to
// delete named for accepting the break; the numbers and names of removed
// fields are reserved and kept over later runs, so a new field cannot take
// them.
func TestGenRunHoldsTheCommittedDefinitionsToTheirNumbers(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	proto := filepath.Join(ggconst.DirPB, "note.proto")
	// fresh commits the definition of the model as first written, for the
	// runs that start from it.
	fresh := func(t *testing.T) {
		t.Helper()
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
		require.NoError(t, os.RemoveAll(ggconst.DirPB))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	}

	t.Run("a field keeps its number", func(t *testing.T) {
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, `pb:"11"`, `pb:"13"`, 1)})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field title of message Note was number 11 and is now 13; keep 11, or replace the field in pb/note.proto by \"reserved 11;\" to accept the break")
	})
	t.Run("a number is never reused", func(t *testing.T) {
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "Title string   'json:\"title\" pb:\"11\"'", "Caption string 'json:\"caption\" pb:\"11\"'", 1)})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field caption of message Note takes number 11, which the field title held; a number is never reused, so give caption a fresh number and let 11 stay reserved, or remove the field title from pb/note.proto to accept the break")
	})
	t.Run("a field keeps a compatible type", func(t *testing.T) {
		// string to bytes is wire compatible; a repeated field turning
		// singular, or a string turning integer, is not.
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "Title string   'json:\"title\" pb:\"11\"'", "Title []byte   'json:\"title\" pb:\"11\"'", 1)})
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "Title string   'json:\"title\" pb:\"11\"'", "Title int64    'json:\"title\" pb:\"11\"'", 1)})
		err := genRunWithOptions(genRunOptions{Quiet: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field title of message Note was bytes and is now int64; a type change breaks the wire, so keep bytes or a type compatible with it, or remove the field from pb/note.proto to accept the break")

		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "Tags  []string 'json:\"tags,omitempty\" pb:\"12\" gorm:\"-\"'", "Tags  string   'json:\"tags,omitempty\" pb:\"12\" gorm:\"-\"'", 1)})
		err = genRunWithOptions(genRunOptions{Quiet: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field tags of message Note was repeated and is now singular; a cardinality change breaks the wire, so keep it repeated, or remove the field from pb/note.proto to accept the break")

		// Bytes back to string is compatible as well.
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	})
	t.Run("a hand-written reservation up to max is honored", func(t *testing.T) {
		fresh(t)
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(proto, []byte(strings.Replace(string(content), "  repeated string tags = 12;\n}", "  repeated string tags = 12;\n\n  reserved 1000 to max;\n}", 1)), 0o600))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		kept, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.Contains(t, string(kept), "reserved 1000 to max;")

		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "\tmodel.Base\n", "\tBody string 'json:\"body\" pb:\"4000\"'\n\n\tmodel.Base\n", 1)})
		err = genRunWithOptions(genRunOptions{Quiet: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field body of message Note takes number 4000, which the file reserves")
	})
	t.Run("removed fields stay reserved", func(t *testing.T) {
		fresh(t)
		withoutTags := strings.Replace(protobufNoteModel, "\tTags  []string 'json:\"tags,omitempty\" pb:\"12\" gorm:\"-\"'\n", "", 1)
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": withoutTags})
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.Contains(t, string(content), "reserved 12;")
		require.Contains(t, string(content), `reserved "tags";`)

		// The reservation outlives the run that made it.
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		again, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.Equal(t, string(content), string(again))

		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(withoutTags, "\tmodel.Base\n", "\tBody string 'json:\"body\" pb:\"12\"'\n\n\tmodel.Base\n", 1)})

		err = genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the field body of message Note takes number 12, which the file reserves; give body a fresh number, or remove the reservation of 12 from pb/note.proto to accept the break")
	})
	t.Run("a dropped map field reserves its number", func(t *testing.T) {
		labeled := strings.Replace(protobufNoteModel, "\tmodel.Base\n", "\tLabels map[string]string 'json:\"labels,omitempty\" pb:\"13\" gorm:\"-\"'\n\n\tmodel.Base\n", 1)
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": labeled})
		require.NoError(t, os.RemoveAll(ggconst.DirPB))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		// The entry message of the map goes with the field: it was never
		// a message a client was built against.
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.Contains(t, string(content), "reserved 13;")
		require.Contains(t, string(content), `reserved "labels";`)
		require.NotContains(t, string(content), "LabelsEntry")
	})
	t.Run("a break is accepted by editing the file as told", func(t *testing.T) {
		fresh(t)
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, `pb:"11"`, `pb:"13"`, 1)})
		require.Error(t, genRunWithOptions(genRunOptions{Quiet: true}))
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		edited := strings.Replace(string(content), "  string title = 11;", "  reserved 11;", 1)
		require.NotEqual(t, string(content), edited)
		require.NoError(t, os.WriteFile(proto, []byte(edited), 0o600))

		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

		kept, err := os.ReadFile(proto)
		require.NoError(t, err)
		require.Contains(t, string(kept), "string title = 13;")
		require.Contains(t, string(kept), "reserved 11;", "the reservation outlives the edit")
	})
}

// writeProtobufProject writes the model files of a project whose models are
// served over gRPC. Single quotes in the sources stand for backquotes.
func writeProtobufProject(t *testing.T, projectDir string, files map[string]string) {
	t.Helper()

	for path, source := range files {
		writeProjectFile(t, filepath.Join(projectDir, filepath.FromSlash(path)), strings.ReplaceAll(source, "'", "`"))
	}
}

// readGenerated reads every .proto and .gen.go file under root, the files
// gg gen writes itself, keyed by its slash-separated path relative to root,
// record/item.proto for the file gg gen writes to pb/record/item.proto; the
// Go files the plugins compile beside them are left out.
func readGenerated(t *testing.T, root string) map[string]string {
	t.Helper()

	files := make(map[string]string)
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (!strings.HasSuffix(path, ".proto") && !strings.HasSuffix(path, ggconst.SuffixGenGo)) {
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
// under the paths they are registered by, tmpapp/note.proto, the way protoc
// would from an import root holding pb/ as a directory named tmpapp, so a
// file protoc would refuse fails the test.
func requireProtosCompile(t *testing.T, files map[string]string) {
	t.Helper()

	// bufbuild/protocompile compiles .proto source the way protoc does,
	// parsing and linking it against the well-known types, without protoc
	// installed: what it accepts, protoc accepts.
	sources := make(map[string]string, len(files))
	names := make([]string, 0, len(files))
	for path, content := range files {
		sources["tmpapp/"+path] = content
		names = append(names, "tmpapp/"+path)
	}
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(sources),
		}),
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

	model.Query
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
	dsl.PatchMany(func() {})
	dsl.Route("items/merge", func() {
		dsl.Create(func() {
			dsl.Service("merge")
			dsl.Payload[*MergeReq]()
			dsl.Result[*MergedItemRsp]()
		})
	})
	dsl.Route("items/:id/seal", func() {
		dsl.Create(func() {
			dsl.Service("seal")
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

// protobufPinModel refers to a type of another directory, the Link of
// model/record/item.go: its definition imports record/item.proto by the
// registered path and its conversions call the record package's.
const protobufPinModel = `package model

import (
	"tmpapp/model/record"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Pin keeps a link of an item under a label.
type Pin struct {
	Label string      'json:"label" pb:"11"'
	Link  record.Link 'json:"link" pb:"12" gorm:"-"'

	model.Base
}

func (Pin) TableName() string { return "pins" }

func (Pin) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("pins")
	dsl.Create(func() {})
	dsl.List(func() {})
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
	Spaced   string         'json:"spaced" pb: "21"'
	Limits   map[string]*int32 'json:"limits" pb:"15" gorm:"-"'
	Slots    []*int32          'json:"slots" pb:"16" gorm:"-"'
	Keyed    map[RejectedKey]string 'json:"keyed" pb:"17" gorm:"-"'
	*RejectedExtra

	model.Base
}

// RejectedExtra is embedded through a pointer.
type RejectedExtra struct {
	Note string 'json:"note" pb:"15"'
}

// Speaker is an interface with methods.
type Speaker interface {
	Speak() string
}

// RejectedKey writes its own text, which keys a JSON object differently
// from the key itself.
type RejectedKey string

func (k RejectedKey) MarshalText() ([]byte, error) { return []byte("k:" + string(k)), nil }

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
			dsl.Service("echo")
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

// protobufUnnamedClashModel declares a Box whose unnamed struct field lid
// would become the message BoxLid, the name of the type its cover field
// refers to.
const protobufUnnamedClashModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Box has a cover and a lid.
type Box struct {
	Cover BoxLid 'json:"cover" pb:"11" gorm:"-"'
	Lid   struct {
		Open bool 'json:"open" pb:"1"'
	} 'json:"lid" pb:"12" gorm:"-"'

	model.Base
}

// BoxLid is named like the message the lid field would get.
type BoxLid struct {
	Open bool 'json:"open" pb:"1"'
}

func (Box) TableName() string { return "boxes" }

func (Box) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("boxes")
	dsl.Create(func() {})
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

	model.Pagination
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

// protobufIgnoredModel declares GRPC() and one action a gst.yaml route
// ignore disables.
const protobufIgnoredModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Silent has nothing gRPC can serve.
type Silent struct {
	Title string 'json:"title" pb:"11"'

	model.Base
}

func (Silent) TableName() string { return "silents" }

func (Silent) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("silents")
	dsl.List(func() {})
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

// protobufShapeModel carries one field of every kind the handlers convert
// beyond the ones of the Record model: gorm's date, time of day, JSON
// document, JSON map and JSON wrapper, a soft-delete time, a JSON number, an
// integer enum, an optional integer, a pointer to a struct, a slice of
// pointers, an array, maps of scalars, of structs and of unnamed structs, a
// slice of and a pointer to an unnamed struct, an optional time, any value,
// bytes, a named slice, the framework's version type, an alias of an
// internal type, gorm's JSON slices of structs and of strings, pointers to
// a slice of strings, to a slice of structs, to a map and to bytes, a
// second JSON wrapper, a map keyed by a narrow integer, an optional JSON
// number, a slice of times and a map of pointers to structs.
const protobufShapeModel = `package model

import (
	"encoding/json"
	"time"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Shape carries one field of every kind the handlers convert.
type Shape struct {
	Date    datatypes.Date                   'json:"date" pb:"11"'
	Clock   datatypes.Time                   'json:"clock" pb:"12"'
	Doc     datatypes.JSON                   'json:"doc,omitempty" pb:"13"'
	Attrs   datatypes.JSONMap                'json:"attrs,omitempty" pb:"14"'
	Options datatypes.JSONType[ShapeOptions] 'json:"options" pb:"15"'
	Audit   ShapeAudit                       'json:"audit" pb:"16" gorm:"-"'
	Amount  json.Number                      'json:"amount" pb:"17"'
	Level   ShapeLevel                       'json:"level" pb:"18"'
	Score   *int                             'json:"score,omitempty" pb:"19"'
	Owner   *ShapeOwner                      'json:"owner,omitempty" pb:"20" gorm:"-"'
	Points  []*ShapePoint                    'json:"points,omitempty" pb:"21" gorm:"-"'
	Grid    [2]int32                         'json:"grid" pb:"22" gorm:"-"'
	Scores  map[string]int                   'json:"scores,omitempty" pb:"23" gorm:"-"'
	ByCode  map[int32]ShapePoint             'json:"by_code,omitempty" pb:"24" gorm:"-"'
	Spans   []struct {
		From int 'json:"from" pb:"1"'
		To   int 'json:"to" pb:"2"'
	} 'json:"spans,omitempty" pb:"25" gorm:"-"'
	Note *struct {
		Text string 'json:"text" pb:"1"'
	} 'json:"note,omitempty" pb:"26" gorm:"-"'
	When  *time.Time 'json:"when,omitempty" pb:"27"'
	Any   any        'json:"any,omitempty" pb:"28" gorm:"-"'
	Blob  []byte     'json:"blob,omitempty" pb:"29"'
	Names ShapeNames 'json:"names,omitempty" pb:"30" gorm:"-"'
	// Version is an alias the framework declares for a type of an internal
	// package, which the handlers spell by the alias.
	Version model.Version 'json:"version,omitempty" gorm:"not null;default:1" pb:"31"'
	// Steps and Words are the JSON slices of gorm, of structs and of strings.
	Steps datatypes.JSONSlice[ShapePoint] 'json:"steps,omitempty" pb:"32"'
	Words datatypes.JSONSlice[string]     'json:"words,omitempty" pb:"33"'
	// Aliases, Corners, Weights and Raw are pointers to slices, to a map
	// and to bytes.
	Aliases *[]string         'json:"aliases,omitempty" pb:"34" gorm:"-"'
	Corners *[]ShapePoint     'json:"corners,omitempty" pb:"35" gorm:"-"'
	Weights *map[string]int32 'json:"weights,omitempty" pb:"36" gorm:"-"'
	Raw     *[]byte           'json:"raw,omitempty" pb:"37" gorm:"-"'
	// RawDoc, Extra and Meta are pointers to JSON types, declared as
	// themselves when decoded.
	RawDoc *json.RawMessage   'json:"raw_doc,omitempty" pb:"38" gorm:"-"'
	Extra  *datatypes.JSON    'json:"extra,omitempty" pb:"39" gorm:"-"'
	Meta   *datatypes.JSONMap 'json:"meta,omitempty" pb:"40" gorm:"-"'
	// Rank and Port are narrower than the int32 and uint32 their fields
	// carry, read back through grpc.Narrow, which refuses what they cannot
	// hold.
	Rank int8   'json:"rank" pb:"45"'
	Port uint16 'json:"port" pb:"46"'
	// Extras is a second JSON wrapper, decoded into a value of its own
	// beside the one of Options.
	Extras datatypes.JSONType[ShapeOptions] 'json:"extras" pb:"47"'
	// Cells is a map of unnamed structs, each decoded whole before it is
	// put in.
	Cells map[string]struct {
		Count int32 'json:"count" pb:"1"'
	} 'json:"cells,omitempty" pb:"48" gorm:"-"'
	// ByRank is keyed by an integer narrower than the int32 the message
	// carries, read back through grpc.Narrow like a field.
	ByRank map[int8]ShapePoint 'json:"by_rank,omitempty" pb:"49" gorm:"-"'
	// Price is an optional JSON number, whose message field may be set to
	// the empty string, which is no number.
	Price *json.Number 'json:"price,omitempty" pb:"50"'
	// Stamps holds times in a repeated field, where the zero time travels
	// as 0001-01-01 and comes back as the zero time.
	Stamps []time.Time 'json:"stamps,omitempty" pb:"51" gorm:"-"'
	// Owners is a map of pointers to structs, whose nil values travel as
	// empty messages.
	Owners map[string]*ShapeOwner 'json:"owners,omitempty" pb:"52" gorm:"-"'
	// Ratio, Factors and Costs hold floats in an optional field, a repeated
	// field and a map, each read back through grpc.Finite, which refuses
	// NaN and the infinities.
	Ratio   *float64           'json:"ratio,omitempty" pb:"53" gorm:"-"'
	Factors []float64          'json:"factors,omitempty" pb:"54" gorm:"-"'
	Costs   map[string]float64 'json:"costs,omitempty" pb:"55" gorm:"-"'
	// Name shadows the Name of the embedded ShapeMeta, which keeps its own
	// key and is selected by its path.
	Name string 'json:"name" pb:"41"'
	ShapeMeta
	// Frame refers to the named type Window while the unnamed struct of the
	// window field becomes a message of its own, named after the field:
	// declared beside Shape rather than inside it, it shadows nothing.
	Frame  Window 'json:"frame" pb:"43" gorm:"-"'
	Window struct {
		Width int32 'json:"width" pb:"1"'
	} 'json:"window" pb:"44" gorm:"-"'

	model.Base
}

// Window is a named type a field of Shape refers to beside the unnamed
// struct field of the same name.
type Window struct {
	Width int32 'json:"width" pb:"1"'
}

// ShapeMeta is embedded in Shape, its Name shadowed by the shape's own.
type ShapeMeta struct {
	Name string 'json:"meta_name" pb:"42" gorm:"-"'
}

// ShapeOptions is kept as a JSON document.
type ShapeOptions struct {
	Color string 'json:"color" pb:"1"'
}

// ShapeAudit records when a shape was removed.
type ShapeAudit struct {
	Removed gorm.DeletedAt 'json:"removed" pb:"1"'
}

// ShapeLevel grades a shape.
type ShapeLevel int

const (
	ShapeLevelLow  ShapeLevel = 1
	ShapeLevelHigh ShapeLevel = 2
)

// ShapeOwner owns a shape.
type ShapeOwner struct {
	Name string 'json:"name" pb:"1"'
}

// ShapePoint is a corner of a shape.
type ShapePoint struct {
	X int32 'json:"x" pb:"1"'
	Y int32 'json:"y" pb:"2"'
}

// ShapeNames lists the names of a shape.
type ShapeNames []string

func (Shape) TableName() string { return "shapes" }

func (Shape) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("shapes")
	dsl.Create(func() {})
	dsl.Get(func() {})
}
`

// protobufConversionTest is the test the golden project runs against its
// generated conversions: every kind of field of the Record, Item and Shape
// models goes through its message and comes back as it went in, the unset
// values staying unset.
const protobufConversionTest = `package pb_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"tmpapp/model"
	"tmpapp/model/record"
	"tmpapp/pb"
	pbrecord "tmpapp/pb/record"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestRecordRoundTrips(t *testing.T) {
	at := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	summary := "short"
	in := &model.Record{
		Title:   "title",
		Status:  model.RecordStatusActive,
		Summary: &summary,
		Tags:    []string{"a", "b"},
		Labels:  map[string]string{"k": "v"},
		Count:   3,
		Ratio:   1.5,
		Enabled: true,
		Payload: []byte("bytes"),
		Raw:     json.RawMessage('{"id":9007199254740993,"n":1.10}'),
		Extra:   map[string]any{"ok": true, "list": []any{"x"}},
		Due:     at,
		Meta:    model.RecordMeta{Author: "author", Score: 2},
	}
	in.Window.From, in.Window.To = "from", "to"
	in.ID, in.CreatedBy, in.CreatedAt = "r-1", "u-1", at

	msg := pb.RecordToProto(in)
	require.Equal(t, "r-1", msg.GetId())
	require.Equal(t, "active", msg.GetStatus())
	require.Equal(t, int64(3), msg.GetCount())
	require.Equal(t, at, msg.GetCreatedAt().AsTime())
	require.Nil(t, msg.UpdatedAt, "the zero time is unset")
	require.Equal(t, "from", msg.GetWindow().GetFrom())
	require.Equal(t, int32(2), msg.GetMeta().GetScore())

	require.Equal(t, []byte(in.Raw), msg.GetRaw(), "a JSON document travels as its bytes, every digit kept")

	out, err := pb.RecordFromProto(msg)
	require.NoError(t, err)
	require.Equal(t, in, out)

	require.Nil(t, pb.RecordToProto(nil))
	none, err := pb.RecordFromProto(nil)
	require.NoError(t, err)
	require.Nil(t, none)
	zero, err := pb.RecordFromProto(&pb.Record{})
	require.NoError(t, err)
	require.Equal(t, &model.Record{}, zero, "an empty message decodes into the zero value")

	_, err = pb.RecordFromProto(&pb.Record{Ratio: math.NaN()})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field \x27ratio\x27", status.Convert(err).Message(), "a number JSON has no spelling for is refused")
}

func TestItemLinksRoundTrip(t *testing.T) {
	in := &record.Item{Content: "c", Links: []record.Link{{URL: "https://a", Title: "A"}, {URL: "https://b"}}}

	msg := pbrecord.ItemToProto(in)
	require.Len(t, msg.GetLinks(), 2)
	require.Equal(t, "https://b", msg.GetLinks()[1].GetUrl())
	out, err := pbrecord.ItemFromProto(msg)
	require.NoError(t, err)
	require.Equal(t, in, out)
	bare, err := pbrecord.ItemFromProto(&pbrecord.Item{})
	require.NoError(t, err)
	require.Nil(t, bare.Links, "no links stay no links")

	merged := pbrecord.MergeRspToProto(&record.MergeRsp{Item: in})
	require.Equal(t, "c", merged.GetItem().GetContent())
	require.Nil(t, pbrecord.MergeRspToProto(&record.MergeRsp{}).Item)
}

func TestShapeRoundTrips(t *testing.T) {
	day := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	when := day.Add(time.Hour)
	score := 7
	ratio := 0.5
	price := json.Number("1.5")
	rawDoc := json.RawMessage('{"b":1}')
	extra := datatypes.JSON('{"c":2}')
	meta := datatypes.JSONMap{"m": float64(1)}
	in := &model.Shape{
		Date:    datatypes.Date(day),
		Clock:   datatypes.Time(90 * time.Minute),
		Doc:     datatypes.JSON('{"a":[1,2]}'),
		Attrs:   datatypes.JSONMap{"deep": map[string]any{"x": float64(1)}},
		Options: datatypes.NewJSONType(model.ShapeOptions{Color: "red"}),
		Audit:   model.ShapeAudit{Removed: gorm.DeletedAt{Time: when, Valid: true}},
		Amount:  json.Number("12.50"),
		Level:   model.ShapeLevelHigh,
		Score:   &score,
		Owner:   &model.ShapeOwner{Name: "owner"},
		Points:  []*model.ShapePoint{{X: 1, Y: 2}, nil},
		Grid:    [2]int32{4, 5},
		Scores:  map[string]int{"a": 1},
		ByCode:  map[int32]model.ShapePoint{3: {X: 3, Y: 4}},
		When:    &when,
		Any:     map[string]any{"n": float64(2)},
		Blob:    []byte{1, 2},
		Names:   model.ShapeNames{"n1"},
		Version: 3,
		Steps:   datatypes.JSONSlice[model.ShapePoint]{{X: 5, Y: 6}},
		Words:   datatypes.JSONSlice[string]{"w"},
		Aliases: &[]string{"a"},
		Corners: &[]model.ShapePoint{{X: 7, Y: 8}},
		Weights: &map[string]int32{"w": 1},
		Raw:     &[]byte{9},
		RawDoc:  &rawDoc,
		Extra:   &extra,
		Meta:    &meta,
		Name:    "outer",
		ShapeMeta: model.ShapeMeta{Name: "inner"},
		Frame:   model.Window{Width: 3},
		Rank:    -3,
		Port:    65000,
		ByRank:  map[int8]model.ShapePoint{-1: {X: 9}},
		Price:   &price,
		Stamps:  []time.Time{{}, day},
		Owners:  map[string]*model.ShapeOwner{"o": {Name: "o"}},
		Ratio:   &ratio,
		Factors: []float64{1.5, 2},
		Costs:   map[string]float64{"c": 0.25},
	}
	in.Window = struct {
		Width int32 'json:"width" pb:"1"'
	}{Width: 4}
	in.Spans = append(in.Spans, struct {
		From int 'json:"from" pb:"1"'
		To   int 'json:"to" pb:"2"'
	}{From: 1, To: 2})
	in.Note = &struct {
		Text string 'json:"text" pb:"1"'
	}{Text: "note"}

	msg := pb.ShapeToProto(in)
	require.Equal(t, day, msg.GetDate().AsTime())
	require.Equal(t, 90*time.Minute, msg.GetClock().AsDuration())
	require.Equal(t, "red", msg.GetOptions().GetColor())
	require.Equal(t, when, msg.GetAudit().GetRemoved().AsTime())
	require.Equal(t, "12.50", msg.GetAmount())
	require.Equal(t, int64(2), msg.GetLevel())
	require.Equal(t, int64(7), msg.GetScore())
	require.Equal(t, []int32{4, 5}, msg.GetGrid())
	require.Equal(t, int64(1), msg.GetScores()["a"])
	require.Equal(t, int32(4), msg.GetByCode()[3].GetY())
	require.Equal(t, int64(2), msg.GetSpans()[0].GetTo())
	require.Equal(t, "note", msg.GetNote().GetText())
	require.Nil(t, msg.GetPoints()[1], "a nil element stays nil")
	require.Equal(t, int64(3), msg.GetVersion())
	require.Equal(t, int32(6), msg.GetSteps()[0].GetY())
	require.Equal(t, []string{"w"}, msg.GetWords())
	require.Equal(t, []string{"a"}, msg.GetAliases())
	require.Equal(t, int32(8), msg.GetCorners()[0].GetY())
	require.Equal(t, int32(1), msg.GetWeights()["w"])
	require.Equal(t, []byte{9}, msg.GetRaw())
	require.Equal(t, "outer", msg.GetName())
	require.Equal(t, "inner", msg.GetMetaName())
	require.Equal(t, int32(3), msg.GetFrame().GetWidth(), "frame is the named type Window")
	require.Equal(t, 0.5, msg.GetRatio())
	require.Equal(t, []float64{1.5, 2}, msg.GetFactors())
	require.Equal(t, 0.25, msg.GetCosts()["c"])
	require.Equal(t, int32(4), msg.GetWindow().GetWidth(), "window is the message of the unnamed struct")
	require.Equal(t, int32(-3), msg.GetRank())
	require.Equal(t, uint32(65000), msg.GetPort())
	require.Equal(t, []byte(in.Doc), msg.GetDoc(), "a JSON document travels as its bytes")
	require.Equal(t, int32(9), msg.GetByRank()[-1].GetX())
	require.Equal(t, "1.5", msg.GetPrice())
	require.True(t, msg.GetStamps()[0].AsTime().IsZero(), "the zero time of an element travels as 0001-01-01, not as nothing")
	require.Equal(t, day, msg.GetStamps()[1].AsTime())
	require.Equal(t, "o", msg.GetOwners()["o"].GetName())

	out, err := pb.ShapeFromProto(msg)
	require.NoError(t, err)
	require.Equal(t, in, out)

	t.Run("the values survive the wire", func(t *testing.T) {
		encoded, err := proto.Marshal(msg)
		require.NoError(t, err)
		decoded := new(pb.Shape)
		require.NoError(t, proto.Unmarshal(encoded, decoded))
		wired, err := pb.ShapeFromProto(decoded)
		require.NoError(t, err)
		expected := *in
		// A nil element or value of a message type has no spelling on the
		// wire: it arrives as an empty message, decoded into the zero value.
		expected.Points = []*model.ShapePoint{{X: 1, Y: 2}, {}}
		require.Equal(t, &expected, wired)
	})

	t.Run("a string that is no valid UTF-8 is written out as the JSON encoder writes it", func(t *testing.T) {
		broken := *in
		broken.Name = "a\xffb"
		msg := pb.ShapeToProto(&broken)
		require.Equal(t, "a�b", msg.GetName())
		_, err := proto.Marshal(msg)
		require.NoError(t, err, "the message encodes, where an invalid string would fail the whole response")
	})

	empty, err := pb.ShapeFromProto(&pb.Shape{})
	require.NoError(t, err)
	require.False(t, empty.Audit.Removed.Valid)
	require.Nil(t, empty.Score)
	require.Nil(t, empty.When)
	require.Nil(t, empty.Note)
	require.Nil(t, empty.Any)
	require.Nil(t, empty.Aliases)
	require.Nil(t, empty.Corners)
	require.Nil(t, empty.Weights)
	require.Nil(t, empty.Raw)
	require.Nil(t, empty.RawDoc)
	require.Nil(t, empty.Extra)
	require.Nil(t, empty.Meta)
	require.True(t, time.Time(empty.Date).IsZero())

	t.Run("a value a field cannot hold is refused", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			msg  *pb.Shape
			want string
		}{
			// \x27 is the apostrophe: a single quote in this source stands
			// for a backtick (see writeProtobufProject).
			{name: "an integer out of range", msg: &pb.Shape{Rank: 300}, want: "invalid value for field \x27rank\x27"},
			{name: "an unsigned integer out of range", msg: &pb.Shape{Port: 70000}, want: "invalid value for field \x27port\x27"},
			{name: "a string that is no JSON number", msg: &pb.Shape{Amount: "abc"}, want: "invalid value for field \x27amount\x27"},
			{name: "an empty optional number", msg: &pb.Shape{Price: proto.String("")}, want: "invalid value for field \x27price\x27"},
			{name: "a document that is no JSON", msg: &pb.Shape{Doc: []byte("{")}, want: "invalid value for field \x27doc\x27"},
			{name: "a pointed-to document that is no JSON", msg: &pb.Shape{RawDoc: []byte("nope")}, want: "invalid value for field \x27raw_doc\x27"},
			{name: "a timestamp past the year 9999", msg: &pb.Shape{When: &timestamppb.Timestamp{Seconds: 253402300800}}, want: "invalid value for field \x27when\x27"},
			{name: "a framework timestamp past the year 9999", msg: &pb.Shape{CreatedAt: &timestamppb.Timestamp{Seconds: 253402300800}}, want: "invalid value for field \x27created_at\x27"},
			{name: "a timestamp in a slice past the year 9999", msg: &pb.Shape{Stamps: []*timestamppb.Timestamp{{Seconds: 253402300800}}}, want: "invalid value for field \x27stamps[0]\x27"},
			{name: "a timestamp inside a nested message past the year 9999", msg: &pb.Shape{Audit: &pb.ShapeAudit{Removed: &timestamppb.Timestamp{Seconds: 253402300800}}}, want: "invalid value for field \x27audit.removed\x27"},
			{name: "a map key out of range", msg: &pb.Shape{ByRank: map[int32]*pb.ShapePoint{300: {}}}, want: "invalid value for field \x27by_rank\x27"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := pb.ShapeFromProto(tt.msg)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				require.Equal(t, tt.want, status.Convert(err).Message())
			})
		}
	})
}

func TestPinRoundTrips(t *testing.T) {
	in := &model.Pin{Label: "label", Link: record.Link{URL: "https://example.test/page", Title: "page"}}
	in.ID = "p-1"

	msg := pb.PinToProto(in)
	require.IsType(t, &pbrecord.Link{}, msg.GetLink(), "the link travels as the message of the record package")
	require.Equal(t, "page", msg.GetLink().GetTitle())

	out, err := pb.PinFromProto(msg)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestConversionsGuardContainerElements(t *testing.T) {
	// A string element or map key with bytes that are no UTF-8 is written
	// through grpc.UTF8 like a string field, so the message marshals and the
	// bytes come out as U+FFFD.
	bad := "a\xffb"
	summary := bad
	record := pb.RecordToProto(&model.Record{Summary: &summary, Tags: []string{bad}, Labels: map[string]string{bad: bad}})
	_, err := proto.Marshal(record)
	require.NoError(t, err)
	require.Equal(t, "a\uFFFDb", record.GetSummary())
	require.Equal(t, []string{"a\uFFFDb"}, record.GetTags())
	require.Equal(t, map[string]string{"a\uFFFDb": "a\uFFFDb"}, record.GetLabels())
	aliases := []string{bad}
	weights := map[string]int32{bad: 1}
	shape := pb.ShapeToProto(&model.Shape{Names: model.ShapeNames{bad}, Words: datatypes.JSONSlice[string]{bad}, Aliases: &aliases, Weights: &weights})
	_, err = proto.Marshal(shape)
	require.NoError(t, err)
	require.Equal(t, []string{"a\uFFFDb"}, shape.GetNames())
	require.Equal(t, []string{"a\uFFFDb"}, shape.GetWords())
	require.Equal(t, []string{"a\uFFFDb"}, shape.GetAliases())
	require.Equal(t, int32(1), shape.GetWeights()["a\uFFFDb"])

	// A float in an optional field, a repeated field or a map is read
	// through grpc.Finite like a float field, so NaN is refused naming the
	// field, the element by its index and the map value by its key.
	nan := math.NaN()
	for _, tt := range []struct {
		field string
		msg   *pb.Shape
	}{
		{"ratio", &pb.Shape{Ratio: &nan}},
		{"factors[1]", &pb.Shape{Factors: []float64{1, nan}}},
		{"costs.c", &pb.Shape{Costs: map[string]float64{"c": nan}}},
	} {
		_, err := pb.ShapeFromProto(tt.msg)
		require.Equal(t, codes.InvalidArgument, status.Code(err), tt.field)
		require.Equal(t, "invalid value for field \x27"+tt.field+"\x27", status.Convert(err).Message(), tt.field)
	}
}
`

// protobufFeedModel declares the three kinds of Stream action, a server
// stream (twice, one on a route with a parameter and without a Payload), a
// client stream on a route with a parameter, and a bidirectional one, on a
// model served over gRPC alone.
const protobufFeedModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Feed is a topic events are published on.
type Feed struct {
	Topic string 'json:"topic" pb:"11"'

	model.Base
}

// FeedWatchReq names the topic to watch.
type FeedWatchReq struct {
	Topic string 'json:"topic" pb:"1"'
}

// FeedEvent is one event of a feed: what every stream carries, each under a
// name of its own.
type FeedEvent struct {
	Seq  int64  'json:"seq" pb:"1"'
	Body string 'json:"body" pb:"2"'
}

// FeedWatchRsp is an event the watch streams out, FeedTailRsp one the tail
// streams out, FeedUploadReq one the upload streams in, FeedChatReq one the
// chat streams in and FeedChatRsp one it streams back, FeedIngestReq one the
// ingest streams in.
type (
	FeedWatchRsp  = FeedEvent
	FeedTailRsp   = FeedEvent
	FeedUploadReq = FeedEvent
	FeedChatReq   = FeedEvent
	FeedChatRsp   = FeedEvent
	FeedIngestReq = FeedEvent
)

// FeedUploadRsp counts the events a client streamed in.
type FeedUploadRsp struct {
	Accepted int64 'json:"accepted" pb:"1"'
}

func (Feed) TableName() string { return "feeds" }

func (Feed) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("feeds")
	dsl.Route("feeds/watch", func() {
		dsl.Stream(func() {
			dsl.Service("watch")
			dsl.Payload[*FeedWatchReq]()
			dsl.StreamingResult[*FeedWatchRsp]()
		})
	})
	dsl.Route("feeds/:feed/tail", func() {
		dsl.Stream(func() {
			dsl.Service("tail")
			dsl.StreamingResult[*FeedTailRsp]()
		})
	})
	dsl.Route("feeds/:feed/upload", func() {
		dsl.Stream(func() {
			dsl.Service("upload")
			dsl.StreamingPayload[*FeedUploadReq]()
			dsl.Result[*FeedUploadRsp]()
		})
	})
	dsl.Route("feeds/chat", func() {
		dsl.Stream(func() {
			dsl.Service("chat")
			dsl.StreamingPayload[*FeedChatReq]()
			dsl.StreamingResult[*FeedChatRsp]()
		})
	})
	dsl.Route("feeds/ingest", func() {
		dsl.Stream(func() {
			dsl.Service("ingest")
			dsl.StreamingPayload[*FeedIngestReq]()
		})
	})
}
`

// protobufDataPackageModel is a model of a package named data, the name of
// the local the generated conversions decode a JSON wrapper into.
const protobufDataPackageModel = `package data

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Sample keeps its options as a JSON document.
type Sample struct {
	Options datatypes.JSONType[SampleOptions] 'json:"options" pb:"11"'

	model.Base
}

// SampleOptions is kept as a JSON document.
type SampleOptions struct {
	Color string 'json:"color" pb:"1"'
}

func (Sample) TableName() string { return "samples" }

func (Sample) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("samples")
	dsl.Create(func() {})
}
`

// protobufUnexportedEmbeddingModel shadows a field of an unexported
// embedded struct with a field of its own, keeping both JSON keys.
const protobufUnexportedEmbeddingModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Entry has a name of its own beside the one of its meta.
type Entry struct {
	Name string 'json:"title" pb:"11"'
	entryMeta

	model.Base
}

type entryMeta struct {
	Name string 'json:"name" pb:"12" gorm:"-"'
}

func (Entry) TableName() string { return "entries" }

func (Entry) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("entries")
	dsl.Create(func() {})
}
`

// protobufSharedDeclarationModel declares two fields together without a
// pb tag.
const protobufSharedDeclarationModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Spot has two coordinates declared together.
type Spot struct {
	X, Y int32

	model.Base
}

func (Spot) TableName() string { return "spots" }

func (Spot) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("spots")
	dsl.Create(func() {})
}
`

// protobufSharedEmbeddedAudit, Note and Record embed one struct without a
// pb tag on its field in two models whose tagged fields end at different
// numbers.
const protobufSharedEmbeddedAudit = `package model

// Audit is embedded in Note and Record.
type Audit struct {
	Reviewer string 'json:"reviewer"'
}
`

const protobufSharedEmbeddedNote = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Note is a note.
type Note struct {
	Title string 'json:"title" pb:"11"'
	Body  string 'json:"body" pb:"12"'
	Audit

	model.Base
}

func (Note) TableName() string { return "notes" }

func (Note) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("notes")
	dsl.Create(func() {})
}
`

const protobufSharedEmbeddedRecord = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is a record.
type Record struct {
	Title string 'json:"title" pb:"11"'
	Audit

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`

// protobufCycleAuthorModel and protobufCycleBookModel refer to each other
// across two files.
const protobufCycleAuthorModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Author writes books.
type Author struct {
	Name  string 'json:"name" pb:"11"'
	Books []Book 'json:"books" pb:"12" gorm:"-"'

	model.Base
}

func (Author) TableName() string { return "authors" }

func (Author) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("authors")
	dsl.Create(func() {})
}
`

const protobufCycleBookModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Book is written by an author.
type Book struct {
	Title  string  'json:"title" pb:"11"'
	Author *Author 'json:"author" pb:"12" gorm:"-"'

	model.Base
}

func (Book) TableName() string { return "books" }

func (Book) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("books")
	dsl.Create(func() {})
}
`

// protobufPBFileModel is a model declared in a file named pb.go.
const protobufPBFileModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Pb is declared in pb.go.
type Pb struct {
	Title string 'json:"title" pb:"11"'

	model.Base
}

func (Pb) TableName() string { return "pbs" }

func (Pb) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("pbs")
	dsl.Create(func() {})
}
`

// protobufUntaggedModel declares fields without pb tags at every level:
// the model, an unnamed struct nested in it and a referenced type.
const protobufUntaggedModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Draft is numbered by gg gen.
type Draft struct {
	Title  string   'json:"title" pb:"11"'
	Body   string   'json:"body"'
	Tags   []string 'json:"tags,omitempty" gorm:"-"' // trailing comment
	Window struct {
		From string 'json:"from"'
		To   string
	} 'json:"window" gorm:"-"'
	Meta DraftMeta 'json:"meta" gorm:"-"'

	model.Base
}

// DraftMeta is kept beside a draft.
type DraftMeta struct {
	Author string 'json:"author"'
	Score  int32  'json:"score" pb:"5"'
	Note   string 'json:"note"'
}

func (Draft) TableName() string { return "drafts" }

func (Draft) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("drafts")
	dsl.Create(func() {})
}
`

// TestGenRunHoldsTheRPCMessagesToTheirCommittedNumbers pins that the
// request and response messages of an rpc take their numbers from the
// definition already under pb/, the way a model's untagged fields do: a
// field the committed message holds keeps its number whatever position it
// holds now, a new field takes the next free number, one the committed
// message reserves excluded, and a field the committed message held stays
// reserved.
func TestGenRunHoldsTheRPCMessagesToTheirCommittedNumbers(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	proto := filepath.Join(ggconst.DirPB, "note.proto")
	// fresh commits the definition as first generated, then rewrites it as
	// edit leaves it, for the run that starts from it.
	fresh := func(t *testing.T, edit func(string) string) {
		t.Helper()
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
		require.NoError(t, os.RemoveAll(ggconst.DirPB))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		edited := edit(string(content))
		require.NotEqual(t, string(content), edited, "the edit has to change the committed file")
		require.NoError(t, os.WriteFile(proto, []byte(edited), 0o600))
	}
	regenerated := func(t *testing.T) string {
		t.Helper()
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		return string(content)
	}
	// inRequest applies edit to the block of the committed file declaring
	// GetNoteRequest, whose fields Note declares alike.
	inRequest := func(edit func(string) string) func(string) string {
		return func(s string) string {
			start := strings.Index(s, "message GetNoteRequest {")
			end := start + strings.Index(s[start:], "\n}\n") + len("\n}\n")
			return s[:start] + edit(s[start:end]) + s[end:]
		}
	}

	t.Run("a field keeps its number by name", func(t *testing.T) {
		// The committed definition numbers two fields the other way round,
		// as a generator ordering the fields differently would have.
		fresh(t, inRequest(func(s string) string {
			s = strings.Replace(s, "  string id = 1;", "  string id = 2;", 1)
			return strings.Replace(s, "  repeated string expand = 2;", "  repeated string expand = 1;", 1)
		}))

		content := regenerated(t)

		require.Contains(t, content, "  string id = 2;")
		require.Contains(t, content, "  repeated string expand = 1;")
		require.Less(t, strings.Index(content, "repeated string expand = 1;"), strings.Index(content, "string id = 2;"), "the fields are printed in the order of their numbers")
	})
	t.Run("a new field takes the next free number", func(t *testing.T) {
		// The committed definition lacks a field the generator adds, as one
		// written before the generator added it would, and reserves the
		// number after its last.
		fresh(t, inRequest(func(s string) string {
			return strings.Replace(s, "  // depth is the depth of the expansion, as the _depth query parameter.\n  uint32 depth = 3;\n", "  reserved 3;\n", 1)
		}))

		content := regenerated(t)

		require.Contains(t, content, "  uint32 depth = 4;")
		require.Contains(t, content, "  reserved 3;")
	})
	t.Run("a field the committed message held stays reserved", func(t *testing.T) {
		fresh(t, inRequest(func(s string) string {
			return strings.Replace(s, "  uint32 depth = 3;\n", "  uint32 depth = 3;\n\n  string legacy = 4;\n", 1)
		}))

		content := regenerated(t)

		require.Contains(t, content, "  reserved 4;")
		require.Contains(t, content, `  reserved "legacy";`)
	})
}

// TestGenRunReadsTheItemsOfAPatchManyWhateverTheirPosition pins that the
// handler of a PatchMany finds the items field of its request by name: the
// committed definition may number the items ahead of a route parameter the
// route took later, and the fields are printed in the order of their
// numbers, so the items are not the last field.
func TestGenRunReadsTheItemsOfAPatchManyWhateverTheirPosition(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{
		"model/record.go":      protobufRecordModel,
		"model/record/item.go": protobufItemModel,
	})
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	proto := filepath.Join(ggconst.DirPB, "record", "item.proto")
	content, err := os.ReadFile(proto)
	require.NoError(t, err)
	committed := string(content)
	const request = "message PatchManyItemRequest {"
	_, rest, found := strings.Cut(committed, request)
	require.True(t, found, "the committed definition declares the PatchMany request")
	block := request + rest[:strings.Index(rest, "\n}\n")+len("\n}\n")]
	edited := strings.Replace(block, "  string record = 1;", "  string record = 2;", 1)
	edited = strings.Replace(edited, "  repeated PatchItemRequest items = 2;", "  repeated PatchItemRequest items = 1;", 1)
	require.NotEqual(t, block, edited, "the edit has to swap the two numbers")
	require.NoError(t, os.WriteFile(proto, []byte(strings.Replace(committed, block, edited, 1)), 0o600))

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))

	content, err = os.ReadFile(proto)
	require.NoError(t, err)
	require.Less(t, strings.Index(string(content), "repeated PatchItemRequest items = 1;"), strings.Index(string(content), "string record = 2;"), "the fields are printed in the order of their numbers")
	handlers, err := os.ReadFile(filepath.Join(ggconst.DirPB, "record", "item"+ggconst.SuffixGenGo))
	require.NoError(t, err)
	require.Contains(t, string(handlers), "item.GetItem()")
	require.Contains(t, string(handlers), `"record": item.GetRecord()`)
	build := exec.Command("go", "build", "./pb/...")
	build.Dir = projectDir
	output, err := build.CombinedOutput()
	require.NoError(t, err, "the generated Go files must build: %s", output)
}

// TestGenRunHoldsTheCommittedServicesAndMessages pins the rest of the
// contract the committed file holds, after Buf's breaking rules: a service,
// an rpc and a message the committed file declares must still be declared,
// and an rpc keeps its request and response messages and its streaming,
// since a client was built against each; every breach names the file to
// delete for accepting the break.
func TestGenRunHoldsTheCommittedServicesAndMessages(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	proto := filepath.Join(ggconst.DirPB, "note.proto")
	// fresh commits the definition of the model as first written, then
	// rewrites it as edit leaves it when there is one.
	fresh := func(t *testing.T, edit func(string) string) {
		t.Helper()
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
		require.NoError(t, os.RemoveAll(ggconst.DirPB))
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		if edit == nil {
			return
		}
		content, err := os.ReadFile(proto)
		require.NoError(t, err)
		edited := edit(string(content))
		require.NotEqual(t, string(content), edited, "the edit has to change the committed file")
		require.NoError(t, os.WriteFile(proto, []byte(edited), 0o600))
	}

	t.Run("an rpc and its messages stay declared", func(t *testing.T) {
		fresh(t, nil)
		writeProtobufProject(t, projectDir, map[string]string{"model/note.go": strings.Replace(protobufNoteModel, "\tdsl.Get(func() {})\n", "", 1)})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the rpc GetNote of service NoteService is gone; a client was built against it, so keep it, or remove it from pb/note.proto to accept the break")
		require.Contains(t, err.Error(), "pb/note.proto: the message GetNoteRequest is gone; a client was built against it, so keep it, or remove it from pb/note.proto to accept the break")
		require.Contains(t, err.Error(), "pb/note.proto: the message GetNoteResponse is gone; a client was built against it, so keep it, or remove it from pb/note.proto to accept the break")
	})
	t.Run("a service stays declared", func(t *testing.T) {
		fresh(t, func(s string) string {
			return strings.Replace(s, "service NoteService {", "service NoteArchiveService {", 1)
		})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the service NoteArchiveService is gone; a client was built against it, so keep it, or remove it from pb/note.proto to accept the break")
	})
	t.Run("an rpc keeps its messages", func(t *testing.T) {
		fresh(t, func(s string) string {
			return strings.Replace(s, "rpc GetNote ( GetNoteRequest ) returns ( GetNoteResponse );", "rpc GetNote ( GetNoteRequest ) returns ( CreateNoteResponse );", 1)
		})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/note.proto: the rpc GetNote of service NoteService answered CreateNoteResponse and now answers GetNoteResponse; a client was built against it, so keep it, or remove the rpc from pb/note.proto to accept the break")
	})
	t.Run("an rpc keeps its streaming", func(t *testing.T) {
		fresh(t, nil)
		writeProtobufProject(t, projectDir, map[string]string{"model/feed.go": protobufFeedModel})
		require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
		writeProtobufProject(t, projectDir, map[string]string{"model/feed.go": strings.Replace(protobufFeedModel, "\t\t\tdsl.Payload[*FeedWatchReq]()\n\t\t\tdsl.StreamingResult[*FeedWatchRsp]()", "\t\t\tdsl.StreamingPayload[*FeedWatchReq]()\n\t\t\tdsl.StreamingResult[*FeedWatchRsp]()", 1)})

		err := genRunWithOptions(genRunOptions{Quiet: true})

		require.Error(t, err)
		require.Contains(t, err.Error(), "pb/feed.proto: the rpc WatchFeed of service FeedService was server streaming and is now bidirectional streaming; a change of streaming breaks the wire, so keep it server streaming, or remove the rpc from pb/feed.proto to accept the break")
	})
}
