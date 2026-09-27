package pb

import (
	"context"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// readBack compiles the printed definitions the way Compile will and checks
// that every type reference of the printed text resolves to the type the
// descriptor names: the printer writes relative names, which protobuf
// resolves from the inside out, so a message nested in the one holding a
// field would shadow a top-level message of its name for that field. The
// generator declares no such message any more (every message is a top-level
// one of its file, and the items of a PatchMany are the requests of a
// Patch), so this is the check that keeps it so: a printed file meaning
// anything but the descriptor is reported, field or rpc by name, with the
// message shadowing the one meant.
func readBack(files []File, want map[string]protoreflect.FileDescriptor) error {
	sources := make(map[string]string, len(files))
	names := make([]string, 0, len(files))
	for _, f := range files {
		name := strings.TrimPrefix(f.Path, ggconst.DirPB+"/")
		sources[name] = f.Content
		names = append(names, name)
	}
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(sources)}),
	}
	got, err := compiler.Compile(context.Background(), names...)
	if err != nil {
		return errors.Wrap(err, "read the printed definitions back")
	}
	for _, fd := range got {
		path := ggconst.DirPB + "/" + fd.Path()
		if err := sameTypes(path, fd.Messages(), want[fd.Path()].Messages()); err != nil {
			return err
		}
		services, meant := fd.Services(), want[fd.Path()].Services()
		for i := range services.Len() {
			methods, meantMethods := services.Get(i).Methods(), meant.Get(i).Methods()
			for j := range methods.Len() {
				m, w := methods.Get(j), meantMethods.Get(j)
				if m.Input().FullName() != w.Input().FullName() || m.Output().FullName() != w.Output().FullName() {
					return errors.Newf("%s: the rpc %s reads (%s) returns (%s), where the definition means (%s) returns (%s)",
						path, m.FullName(), m.Input().FullName(), m.Output().FullName(), w.Input().FullName(), w.Output().FullName())
				}
			}
		}
	}
	return nil
}

// sameTypes compares the type every field of got resolves to with the one
// the field of want it corresponds to names, the nested messages included.
func sameTypes(path string, got, want protoreflect.MessageDescriptors) error {
	for i := range got.Len() {
		g := got.Get(i)
		w := want.ByName(g.Name())
		for j := range g.Fields().Len() {
			f := g.Fields().Get(j)
			m := w.Fields().ByNumber(f.Number())
			if f.Message() != nil && f.Message().FullName() != m.Message().FullName() {
				return shadowed(path, f.FullName(), f.Message(), m.Message().FullName())
			}
			if f.Enum() != nil && f.Enum().FullName() != m.Enum().FullName() {
				return shadowed(path, f.FullName(), f.Enum(), m.Enum().FullName())
			}
		}
		if err := sameTypes(path, g.Messages(), w.Messages()); err != nil {
			return err
		}
	}
	return nil
}

// shadowed reports the field of path that reads as got, where the
// definition means meant.
func shadowed(path string, field protoreflect.FullName, got protoreflect.Descriptor, meant protoreflect.FullName) error {
	where := "a top-level message"
	if parent, ok := got.Parent().(protoreflect.MessageDescriptor); ok {
		where = "the message nested in " + string(parent.FullName())
	}
	return errors.Newf("%s: the field %s reads as %s, %s, where the definition means %s: the nested message shadows it; name the field or the type differently", path, field, got.FullName(), where, meant)
}
