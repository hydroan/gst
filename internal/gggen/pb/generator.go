package pb

import (
	"go/types"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/hydroan/gst/internal/modelinfo"
	"google.golang.org/protobuf/types/descriptorpb"
)

// generator walks the models declaring GRPC() and the project types their
// services reach, and builds the descriptor of every .proto file.
type generator struct {
	cfg     Config
	project *jsonshape.Project
	appName string // the last element of the module path, prefix of every protobuf package
	models  []*modelinfo.Model

	files map[string]*protoFile // the files built so far, keyed by their name relative to pb/
	// messages holds an entry for every project type given a message so far;
	// the entry is filled when the type is queued and its fields are built
	// when it is dequeued. byFullName holds the same entries by the name a
	// field type refers to the message by.
	messages   map[*types.TypeName]*message
	byFullName map[string]*message
	queue      []*types.TypeName
	// goNames and protoNames hold the Go type name and the name in the file
	// of every message descriptor built, RecordWindow both for the message
	// of the window field of Record, which name the messages of the unnamed
	// struct fields below and find the committed message to hold to.
	goNames    map[*descriptorpb.DescriptorProto]string
	protoNames map[*descriptorpb.DescriptorProto]string
	// committedFiles caches the definitions already under pb/ (see committed).
	committedFiles map[string]committedFile
	// missingTags lists the fields numbered for want of a pb tag (see
	// MissingTag).
	missingTags []MissingTag
}

// message is the message of one project type: the file it is declared in,
// its name there and its conversion, filled when its fields are built.
type message struct {
	obj  *types.TypeName
	file *protoFile
	name string // the message name, Sample
	conv *conversion
}

// fullName is the fully-qualified reference to the message, as a field type
// names it: .app.model.Sample.
func (m *message) fullName() string { return "." + m.file.pkg + "." + m.name }

// protoFile is one .proto file being built: the descriptor of its package,
// imports and options, and the messages and services it declares together
// with their comments; and what the Go file generated beside it serves, the
// rpcs of its services and the project types its messages convert.
type protoFile struct {
	name string // the file name relative to pb/, archive/document.proto
	// registered is the path the definition is compiled and registered
	// under at run time, the application's name followed by the file's path
	// under pb/, tmpapp/archive/document.proto: the directory the package
	// names, as Buf's PACKAGE_DIRECTORY_MATCH rule has it and as googleapis
	// lays its files out, so a consumer of the definitions copies pb/ into
	// its import root as a directory of the application's name; and it
	// keeps the definitions of one application apart from another's
	// board/feed.proto when both are linked into one binary, the protobuf
	// registry refusing a path registered twice.
	registered string
	pkg        string // the protobuf package, app.archive
	goPackage  string // the go_package option, example.com/app/pb/archive;archive
	imports    map[string]bool
	messages   []*descriptorpb.DescriptorProto
	services   []*descriptorpb.ServiceDescriptorProto
	locations  []*descriptorpb.SourceCodeInfo_Location
	// names are the message names declared so far, to refuse a second
	// declaration of one.
	names map[string]string // message name -> what declared it
	typed []*message        // the messages of project types, in order
	rpcs  []*rpc            // the rpcs of the services, in order
}

// goImportPath and goPackageName are the two halves of the go_package
// option: the import path of the Go package the file's messages are
// compiled into and the name of that package.
func (f *protoFile) goImportPath() string {
	importPath, _, _ := strings.Cut(f.goPackage, ";")
	return importPath
}

func (f *protoFile) goPackageName() string {
	_, name, _ := strings.Cut(f.goPackage, ";")
	return name
}

// dir is the directory of the file relative to pb/, "." for the root, which
// decides the Go package the file and the ones beside it belong to.
func (f *protoFile) dir() string { return path.Dir(f.name) }

// newGenerator prepares a generation run of cfg over the loaded project for
// the models declaring GRPC().
func newGenerator(cfg Config, project *jsonshape.Project, models []*modelinfo.Model) *generator {
	return &generator{
		cfg:            cfg,
		project:        project,
		appName:        modelinfo.AppName(cfg.ModulePath),
		models:         models,
		files:          make(map[string]*protoFile),
		messages:       make(map[*types.TypeName]*message),
		byFullName:     make(map[string]*message),
		goNames:        make(map[*descriptorpb.DescriptorProto]string),
		protoNames:     make(map[*descriptorpb.DescriptorProto]string),
		committedFiles: make(map[string]committedFile),
	}
}

// generate declares the service of every model, builds the messages they
// reach and prints the files, the definitions and the Go files serving
// them, or reports every diagnostic found on the way. The messages of the
// models are built first, ahead of every type they reach, so that a file
// opens with its own model's message whichever model reaches a type of it
// first: the Pin of the golden fixture, declared before Item, refers to
// Item's Link, which would otherwise be built into item.proto ahead of
// Item. The model a service cannot be declared for is reported by
// declareService, not here.
func (g *generator) generate() ([]File, error) {
	for _, m := range g.models {
		if obj := g.modelType(m); obj != nil && !m.Design.IsEmpty {
			g.messageOf(obj)
		}
	}
	g.buildQueued()
	for _, m := range g.models {
		g.declareService(m)
	}
	g.buildQueued()
	// The handlers of a definition go beside it under the same name, and
	// pb.gen.go in every directory is the registration file's (see
	// registrationFiles).
	for _, name := range slices.Sorted(maps.Keys(g.files)) {
		if path.Base(name) == ggconst.DirPB+".proto" {
			dir := path.Dir(name)
			g.project.Report(jsonshape.Site{Subject: ggconst.DirPB + "/" + name},
				"the model file %s would get its handlers at %s, the registration file; rename the file",
				path.Join(ggconst.DirModel, dir, ggconst.DirPB+ggconst.ExtensionGo), path.Join(ggconst.DirPB, dir, ggconst.FilePBGen))
		}
	}
	// The plugin writes the service of x.proto to x_grpc.pb.go, where the
	// messages of x_grpc.proto would go too.
	for _, name := range slices.Sorted(maps.Keys(g.files)) {
		if stem, ok := strings.CutSuffix(name, "_grpc.proto"); ok {
			g.project.Report(jsonshape.Site{Subject: ggconst.DirPB + "/" + name},
				"the model file %s/%s_grpc.go ends in _grpc, the suffix of the service file the protobuf plugin writes for %s/%s.go; rename the file", ggconst.DirModel, stem, ggconst.DirModel, stem)
		}
	}
	if diags := g.project.Diagnostics(); len(diags) > 0 {
		return nil, &DiagnosticsError{Diagnostics: diags, MissingTags: g.missingTags}
	}
	g.reconcile()
	if diags := g.project.Diagnostics(); len(diags) > 0 {
		return nil, &DiagnosticsError{Diagnostics: diags}
	}
	files, err := g.print()
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(g.files)) {
		var handlers File
		if handlers, err = g.handlerFile(g.files[name]); err != nil {
			return nil, err
		}
		files = append(files, handlers)
	}
	registrations, err := g.registrationFiles()
	if err != nil {
		return nil, err
	}
	files = append(files, registrations...)
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
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

// modelType returns the type of the model m in the loaded project, nil when
// the project holds no such package or type; declareService reports either
// case.
func (g *generator) modelType(m *modelinfo.Model) *types.TypeName {
	pkg := g.project.Package(m.ImportPath())
	if pkg == nil || pkg.Types == nil {
		return nil
	}
	obj, _ := pkg.Types.Scope().Lookup(m.ModelName).(*types.TypeName)
	return obj
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
	g.byFullName[m.fullName()] = m
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
// package and go_package its directory implies and the path it is registered
// under.
func (g *generator) file(name string) *protoFile {
	if f, ok := g.files[name]; ok {
		return f
	}
	dir := path.Dir(name)
	f := &protoFile{
		name:       name,
		registered: path.Join(g.appName, name),
		pkg:        protoPackage(g.appName, dir),
		goPackage:  goPackageOption(g.cfg.ModulePath, dir),
		imports:    make(map[string]bool),
		names:      make(map[string]string),
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

// messageNamed returns the top-level message of the file named name, nil
// when the file declares none.
func (f *protoFile) messageNamed(name string) *descriptorpb.DescriptorProto {
	for _, m := range f.messages {
		if m.GetName() == name {
			return m
		}
	}
	return nil
}

// addService appends a service to the file under its leading comment.
func (f *protoFile) addService(s *descriptorpb.ServiceDescriptorProto, comment string) {
	f.comment([]int32{fileServicesTag, int32Index(len(f.services))}, comment)
	f.services = append(f.services, s)
}

// The field numbers of FileDescriptorProto and its children that source
// locations are addressed by, as in descriptor.proto.
const (
	fileSyntaxTag         int32 = 12
	fileMessagesTag       int32 = 4
	fileServicesTag       int32 = 6
	messageFieldsTag      int32 = 2
	messageNestedTypesTag int32 = 3
	serviceMethodsTag     int32 = 2
	fieldMaxNumber        int32 = 536870911
	reservedRangeStart    int32 = 19000
	reservedRangeEnd      int32 = 19999
	syntheticOneofPrefix        = "_"
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

// importOf makes the file import the file dependency, a registered path,
// unless it is the file itself.
func (f *protoFile) importOf(dependency string) {
	if dependency != f.registered {
		f.imports[dependency] = true
	}
}
