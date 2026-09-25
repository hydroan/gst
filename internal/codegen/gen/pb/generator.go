package pb

import (
	"go/types"
	"path"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/codegen/gen/jsonshape"
	"github.com/hydroan/gst/internal/ggconst"
	"google.golang.org/protobuf/types/descriptorpb"
)

// generator walks the models declaring GRPC() and the project types their
// services reach, and builds the descriptor of every .proto file.
type generator struct {
	cfg     Config
	project *jsonshape.Project
	appName string // the last element of the module path, prefix of every protobuf package
	models  []*gen.ModelInfo

	files map[string]*protoFile // the files built so far, keyed by their name relative to pb/
	// messages holds an entry for every project type given a message so far;
	// the entry is filled when the type is queued and its fields are built
	// when it is dequeued.
	messages map[*types.TypeName]*message
	queue    []*types.TypeName
}

// message is the message of one project type: the file it is declared in and
// its name there.
type message struct {
	obj  *types.TypeName
	file *protoFile
	name string // the message name, Sample
}

// fullName is the fully-qualified reference to the message, as a field type
// names it: .app.model.Sample.
func (m *message) fullName() string { return "." + m.file.pkg + "." + m.name }

// protoFile is one .proto file being built: the descriptor of its package,
// imports and options, and the messages and services it declares together
// with their comments.
type protoFile struct {
	name      string // the file name relative to pb/, archive/document.proto
	pkg       string // the protobuf package, app.archive
	goPackage string // the go_package option, example.com/app/pb/archive;archive
	imports   map[string]bool
	messages  []*descriptorpb.DescriptorProto
	services  []*descriptorpb.ServiceDescriptorProto
	locations []*descriptorpb.SourceCodeInfo_Location
	// names are the message names declared so far, to refuse a second
	// declaration of one.
	names map[string]string // message name -> what declared it
}

// newGenerator prepares a generation run of cfg over the loaded project for
// the models declaring GRPC().
func newGenerator(cfg Config, project *jsonshape.Project, models []*gen.ModelInfo) *generator {
	return &generator{
		cfg:      cfg,
		project:  project,
		appName:  path.Base(cfg.ModulePath),
		models:   models,
		files:    make(map[string]*protoFile),
		messages: make(map[*types.TypeName]*message),
	}
}

// generate declares the service of every model, builds the messages they
// reach and prints the files, or reports every diagnostic found on the way.
func (g *generator) generate() ([]File, error) {
	for _, m := range g.models {
		g.declareService(m)
	}
	g.buildQueued()
	if diags := g.project.Diagnostics(); len(diags) > 0 {
		return nil, &DiagnosticsError{Diagnostics: diags}
	}
	return g.print()
}

// buildQueued builds the fields of every queued message, and of the messages
// those reach.
func (g *generator) buildQueued() {
	for len(g.queue) > 0 {
		obj := g.queue[0]
		g.queue = g.queue[1:]
		g.buildMessage(obj)
	}
}

// messageOf returns the message of a project type, queueing the type to have
// its fields built once. The message goes to the file mirroring the Go file
// the type is declared in.
func (g *generator) messageOf(obj *types.TypeName) *message {
	if m, ok := g.messages[obj]; ok {
		return m
	}
	file := g.fileOf(obj)
	m := &message{obj: obj, file: file, name: obj.Name()}
	if holder, ok := file.claim(obj.Name(), "the type "+obj.Pkg().Path()+"."+obj.Name()); !ok {
		g.project.Report(jsonshape.Site{Subject: obj.Pkg().Path() + "." + obj.Name(), Pos: obj.Pos()},
			"the message %s clashes with %s; rename the type", obj.Name(), holder)
	}
	g.messages[obj] = m
	g.queue = append(g.queue, obj)
	return m
}

// fileOf returns the file mirroring the Go file obj is declared in, creating
// it on first use: model/archive/document.go maps to archive/document.proto
// under pb/, and a file outside the model directory, such as
// pkg/notifier/notifier.go, keeps its path relative to the project root.
func (g *generator) fileOf(obj *types.TypeName) *protoFile {
	goFile := g.project.RelativeFile(g.project.FileSet().Position(obj.Pos()).Filename)
	name := filepath.ToSlash(strings.TrimSuffix(goFile, ggconst.ExtensionGo)) + ".proto"
	name = strings.TrimPrefix(name, ggconst.DirModel+"/")
	return g.file(name)
}

// file returns the file of the given name, creating it on first use with the
// package and go_package its directory implies.
func (g *generator) file(name string) *protoFile {
	if f, ok := g.files[name]; ok {
		return f
	}
	dir := path.Dir(name)
	f := &protoFile{
		name:      name,
		pkg:       protoPackage(g.appName, dir),
		goPackage: goPackageOption(g.cfg.ModulePath, dir),
		imports:   make(map[string]bool),
		names:     make(map[string]string),
	}
	g.files[name] = f
	return f
}

// claim records name as owner's, the type or rpc the name stands for ("the
// type app/model.Item", "the rpc ItemService.Create"), and reports whether the
// name is free or already owner's. A name another owner holds stays theirs,
// and claim returns that holder for the diagnostic naming the clash.
func (f *protoFile) claim(name, owner string) (holder string, ok bool) {
	if previous, taken := f.names[name]; taken {
		return previous, previous == owner
	}
	f.names[name] = owner
	return owner, true
}

// addMessage appends a message to the file under its leading comment.
func (f *protoFile) addMessage(m *descriptorpb.DescriptorProto, comment string) {
	f.comment([]int32{fileMessagesTag, int32Index(len(f.messages))}, comment)
	f.messages = append(f.messages, m)
}

// addService appends a service to the file under its leading comment.
func (f *protoFile) addService(s *descriptorpb.ServiceDescriptorProto, comment string) {
	f.comment([]int32{fileServicesTag, int32Index(len(f.services))}, comment)
	f.services = append(f.services, s)
}

// The field numbers of FileDescriptorProto and its children that source
// locations are addressed by, as in descriptor.proto.
const (
	fileSyntaxTag        int32 = 12
	fileMessagesTag      int32 = 4
	fileServicesTag      int32 = 6
	messageFieldsTag     int32 = 2
	messageNestedTag     int32 = 3
	serviceMethodsTag    int32 = 2
	fieldMaxNumber       int32 = 536870911
	reservedRangeStart   int32 = 19000
	reservedRangeEnd     int32 = 19999
	syntheticOneofPrefix       = "_"
)

// int32Index converts the index of a descriptor list element to the int32
// the source locations address it by.
func int32Index(n int) int32 {
	return int32(n) //nolint:gosec // The lists of a file are far below the int32 range.
}

// comment records the source location of the element at path with its
// leading comment, in the form protoc records comments: each line with a
// leading space and a trailing newline. Every element gets a location, with
// or without a comment: the printer lays elements out in the order of their
// locations, which is the order they were recorded in, and puts elements
// without one last.
func (f *protoFile) comment(path []int32, text string) {
	location := &descriptorpb.SourceCodeInfo_Location{Path: path, Span: []int32{0, 0, 0}}
	if text != "" {
		location.LeadingComments = new(commentText(text))
	}
	f.locations = append(f.locations, location)
}

// commentText renders text the way protoc records a comment, a space before
// every line and a newline after it, a blank line kept bare: " Title is the
// display title.\n" for the one-line doc "Title is the display title.", and
// " Title is the display title.\n\n Two lines.\n" for the two-paragraph doc
// "Title is the display title.\n\nTwo lines.\n".
func commentText(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(" ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// importOf makes the file import the file dependency, unless it is the file
// itself.
func (f *protoFile) importOf(dependency string) {
	if dependency != f.name {
		f.imports[dependency] = true
	}
}
