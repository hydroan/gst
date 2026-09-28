package pb

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// The two protobuf plugins Compile drives, at the versions the framework's
// go.mod requires of the modules providing them, which a test keeps them
// equal to: protoc-gen-go writes the messages of a file, protoc-gen-go-grpc
// its service. They run through gghelper.PinnedCommand, the way gg lint
// runs golangci-lint: built apart from the project's module and cached, so
// nothing is installed and the project's go.mod names nothing for them, and
// the Go files match the protobuf and grpc runtimes the project gets through
// the framework.
const (
	protocGenGoModule      = "google.golang.org/protobuf"
	protocGenGoPackage     = "google.golang.org/protobuf/cmd/protoc-gen-go"
	protocGenGoVersion     = "v1.36.12"
	protocGenGoGRPCModule  = "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	protocGenGoGRPCPackage = "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	protocGenGoGRPCVersion = "v1.6.2"
)

// Compile compiles the .proto files Generate produced into the Go files the
// protobuf plugins write for them, each beside its definition: for
// pb/sample.proto, pb/sample.pb.go with its messages and, when the file
// declares a service, pb/sample_grpc.pb.go with the service; for
// pb/record/item.proto, pb/record/item.pb.go in the package its go_package
// names. The definitions are compiled in memory, with protocompile standing
// in for protoc and carrying the well-known types they import, so nothing
// has to be on disk first and nothing is written when a definition or a
// plugin refuses; the plugins get the descriptors the way protoc hands them
// over, comments included. The files come back sorted by path.
func Compile(modulePath string, protos []File) ([]File, error) {
	if len(protos) == 0 {
		return nil, nil
	}
	// The definitions are compiled under the paths they are registered
	// under at run time, the application's name followed by their path
	// under pb/, tmpapp/sample.proto (see protoFile.registered): the path
	// they import one another by, and the one the compiler and the plugins
	// see; the plugins' outputs are named the same way, and come back under
	// pb/.
	app := modelinfo.AppName(modulePath)
	sources := make(map[string]string, len(protos))
	names := make([]string, 0, len(protos))
	for _, f := range protos {
		name := path.Join(app, strings.TrimPrefix(f.Path, ggconst.DirPB+"/"))
		sources[name] = f.Content
		names = append(names, name)
	}
	slices.Sort(names)
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(sources),
		}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	linked, err := compiler.Compile(context.Background(), names...)
	if err != nil {
		return nil, errors.Wrap(err, "compile the protobuf definitions")
	}

	// Every file the plugins may look at, each after the files it imports,
	// the order protoc keeps.
	var descriptors []*descriptorpb.FileDescriptorProto
	seen := make(map[string]bool)
	var collect func(fd protoreflect.FileDescriptor)
	collect = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		imports := fd.Imports()
		for i := range imports.Len() {
			collect(imports.Get(i).FileDescriptor)
		}
		descriptors = append(descriptors, protodesc.ToFileDescriptorProto(fd))
	}
	for _, f := range linked {
		collect(f)
	}
	request := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: names,
		// The output of a definition goes beside it, pb/record/item.pb.go
		// for record/item.proto, not under its go_package's import path.
		Parameter: new("paths=source_relative"),
		ProtoFile: descriptors,
	}

	var files []File
	for _, plugin := range []struct{ module, version, pkg string }{
		{protocGenGoModule, protocGenGoVersion, protocGenGoPackage},
		{protocGenGoGRPCModule, protocGenGoGRPCVersion, protocGenGoGRPCPackage},
	} {
		generated, err := runPlugin(plugin.module, plugin.version, plugin.pkg, request)
		if err != nil {
			return nil, err
		}
		files = append(files, generated...)
	}
	for i := range files {
		files[i].Path = path.Join(ggconst.DirPB, strings.TrimPrefix(files[i].Path, app+"/"))
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// runPlugin runs the plugin pkg, of the module named by module, at version
// (see gghelper.PinnedCommand), hands it request on its standard input the
// way protoc does and returns the files it answers with, named as the
// plugin names them, after the definitions they come from.
func runPlugin(module, version, pkg string, request *pluginpb.CodeGeneratorRequest) ([]File, error) {
	input, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "encode the plugin request")
	}
	target := pkg + "@" + version
	cmd, err := gghelper.PinnedCommand(module, version, pkg)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "run %s: %s", target, strings.TrimSpace(stderr.String()))
	}
	var response pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, errors.Wrapf(err, "decode the answer of %s", target)
	}
	if response.Error != nil {
		return nil, errors.Newf("%s: %s", target, response.GetError())
	}
	files := make([]File, 0, len(response.GetFile()))
	for _, f := range response.GetFile() {
		files = append(files, File{Path: f.GetName(), Content: f.GetContent()})
	}
	return files, nil
}
