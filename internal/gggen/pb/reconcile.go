package pb

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file holds the definitions to the contract already on disk: the
// committed .proto files are what the clients were built against, so a
// generated file replaces one only if every field keeps its number and no
// number changes hands, and the fields the models dropped stay reserved.

// reconcile reads, for every file about to be written, the committed one
// under pb/ in the project and holds the new file to it, message by message
// (see holdMessage). A committed file that cannot be parsed is reported, so
// that it is never overwritten blindly; a file without a committed
// counterpart is new and free.
func (g *generator) reconcile() {
	for name, f := range g.files {
		s := jsonshape.Site{Subject: ggconst.DirPB + "/" + name}
		path := filepath.Join(g.cfg.Dir, ggconst.DirPB, filepath.FromSlash(name))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		committed, err := readCommitted(path)
		if err != nil {
			g.project.Report(s, "the committed file cannot be read: %v; fix it or delete it", err)
			continue
		}
		old := make(map[string]*descriptorpb.DescriptorProto)
		indexMessages(committed.GetMessageType(), "", old)
		for _, m := range f.messages {
			g.holdMessage(s, m, "", old)
		}
	}
}

// readCommitted parses the .proto file at path on its own, imports
// unresolved, which is all the comparison needs: the messages, their fields
// with numbers, and what they reserve.
func readCommitted(path string) (*descriptorpb.FileDescriptorProto, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	handler := reporter.NewHandler(nil)
	node, err := parser.Parse(path, file, handler)
	if err != nil {
		return nil, err
	}
	result, err := parser.ResultFromAST(node, true, handler)
	if err != nil {
		return nil, err
	}
	return result.FileDescriptorProto(), nil
}

// indexMessages maps every message of messages, and of their nested
// messages, by its dotted name below the file: Record, Record.Window.
func indexMessages(messages []*descriptorpb.DescriptorProto, parent string, index map[string]*descriptorpb.DescriptorProto) {
	for _, m := range messages {
		name := m.GetName()
		if parent != "" {
			name = parent + "." + name
		}
		index[name] = m
		indexMessages(m.GetNestedType(), name, index)
	}
}

// holdMessage holds the message msg, named below parent, to the committed
// message of the same name in old, when there is one: a field both hold
// keeps its number, its cardinality and a wire-compatible type (see
// wireType), a number a field of the committed message held cannot pass to
// a field of another name, and neither can a number or a name the committed
// message reserves be taken; each breach is reported with the file to delete
// for accepting the break. The numbers and names of the
// committed fields the message dropped are reserved in msg, along with
// everything the committed message reserved, so that the next generation
// keeps reserving them; a field coming back under a reserved number or name
// is refused the same way, the file being the one place to lift a
// reservation. Nested messages are held the same way. A Note that held
// title = 11 and tags = 12 and now declares title alone gets
//
//	reserved 12;
//
//	reserved "tags";
func (g *generator) holdMessage(s jsonshape.Site, msg *descriptorpb.DescriptorProto, parent string, old map[string]*descriptorpb.DescriptorProto) {
	name := msg.GetName()
	if parent != "" {
		name = parent + "." + name
	}
	if committed, ok := old[name]; ok {
		numbers := make(map[int32]string, len(committed.GetField()))
		names := make(map[string]int32, len(committed.GetField()))
		for _, f := range committed.GetField() {
			numbers[f.GetNumber()] = f.GetName()
			names[f.GetName()] = f.GetNumber()
		}
		// The ranges stay ranges: a hand-written "reserved 1000 to max;"
		// spans half a billion numbers.
		reserved := slices.Clone(committed.GetReservedRange())
		reservedNames := make(map[string]bool, len(committed.GetReservedName()))
		for _, n := range committed.GetReservedName() {
			reservedNames[n] = true
		}
		fields := make(map[string]*descriptorpb.FieldDescriptorProto, len(committed.GetField()))
		for _, f := range committed.GetField() {
			fields[f.GetName()] = f
		}
		current := make(map[string]bool, len(msg.GetField()))
		for _, f := range msg.GetField() {
			current[f.GetName()] = true
			was, known := fields[f.GetName()]
			switch holder, held := numbers[f.GetNumber()]; {
			case known && was.GetNumber() != f.GetNumber():
				g.project.Report(s, "the field %s of message %s was number %d and is now %d; keep %d, or delete %s to accept the break", f.GetName(), name, was.GetNumber(), f.GetNumber(), was.GetNumber(), s.Subject)
			case known && repeated(was) != repeated(f):
				g.project.Report(s, "the field %s of message %s was %s and is now %s; a cardinality change breaks the wire, so keep it %s, or delete %s to accept the break", f.GetName(), name, cardinality(was), cardinality(f), cardinality(was), s.Subject)
			case known && wireType(was) != wireType(f):
				g.project.Report(s, "the field %s of message %s was %s and is now %s; a type change breaks the wire, so keep %s or a type compatible with it, or delete %s to accept the break", f.GetName(), name, typeName(was), typeName(f), typeName(was), s.Subject)
			case held && holder != f.GetName():
				g.project.Report(s, "the field %s of message %s takes number %d, which the field %s held; a number is never reused, so give %s a fresh number and let %d stay reserved, or delete %s to accept the break", f.GetName(), name, f.GetNumber(), holder, f.GetName(), f.GetNumber(), s.Subject)
			case reserves(reserved, f.GetNumber()):
				g.project.Report(s, "the field %s of message %s takes number %d, which the file reserves; give %s a fresh number, or delete %s to accept the break", f.GetName(), name, f.GetNumber(), f.GetName(), s.Subject)
			case reservedNames[f.GetName()]:
				g.project.Report(s, "the field %s of message %s takes a name the file reserves; a removed field's name is never reused, so name it differently, or delete %s to accept the break", f.GetName(), name, s.Subject)
			}
		}
		for _, f := range committed.GetField() {
			if !current[f.GetName()] {
				reserved = append(reserved, &descriptorpb.DescriptorProto_ReservedRange{Start: new(f.GetNumber()), End: new(f.GetNumber() + 1)})
				reservedNames[f.GetName()] = true
			}
		}
		msg.ReservedRange = mergeRanges(reserved)
		msg.ReservedName = slices.Sorted(func(yield func(string) bool) {
			for n := range reservedNames {
				if !yield(n) {
					return
				}
			}
		})
	}
	for _, nested := range msg.GetNestedType() {
		g.holdMessage(s, nested, name, old)
	}
}

// reserves reports whether the ranges, ends exclusive as the descriptor has
// them, cover the number n.
func reserves(ranges []*descriptorpb.DescriptorProto_ReservedRange, n int32) bool {
	for _, r := range ranges {
		if r.GetStart() <= n && n < r.GetEnd() {
			return true
		}
	}
	return false
}

// mergeRanges sorts the ranges a message reserves and joins the ones that
// touch or overlap, ends exclusive as the descriptor has them: 12 to 13
// (end 14), 13 to 13 and 20 to 20 give 12 to 13 and 20. Ranges never
// expand into numbers, a hand-written "reserved 1000 to max;" spanning half
// a billion of them.
func mergeRanges(ranges []*descriptorpb.DescriptorProto_ReservedRange) []*descriptorpb.DescriptorProto_ReservedRange {
	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, func(a, b *descriptorpb.DescriptorProto_ReservedRange) int {
		return int(a.GetStart() - b.GetStart())
	})
	var merged []*descriptorpb.DescriptorProto_ReservedRange
	for _, r := range sorted {
		if last := len(merged) - 1; last >= 0 && r.GetStart() <= merged[last].GetEnd() {
			if r.GetEnd() > merged[last].GetEnd() {
				merged[last].End = new(r.GetEnd())
			}
			continue
		}
		merged = append(merged, &descriptorpb.DescriptorProto_ReservedRange{Start: new(r.GetStart()), End: new(r.GetEnd())})
	}
	return merged
}

// repeated reports whether the field is repeated; a map field is, being a
// repeated entry message.
func repeated(f *descriptorpb.FieldDescriptorProto) bool {
	return f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
}

// cardinality names the field's cardinality for a diagnostic: repeated or
// singular.
func cardinality(f *descriptorpb.FieldDescriptorProto) string {
	if repeated(f) {
		return "repeated"
	}
	return "singular"
}

// wireType keys the field's type by what it puts on the wire, so that a
// change between compatible types passes: int32, int64, uint32, uint64 and
// bool encode alike, as do sint32 and sint64, string and bytes, fixed32 and
// sfixed32, fixed64 and sfixed64; every other type, a message or an enum
// among them, keys by its own name. A committed file is read unlinked, so a
// named type comes as written there, Link or google.protobuf.Timestamp, and
// a generated one fully qualified, .app.record.Link: both key by the last
// element.
func wireType(f *descriptorpb.FieldDescriptorProto) string {
	if f.GetTypeName() != "" {
		return "named:" + typeName(f)
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "varint"
	case descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return "zigzag"
	case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "bytes"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "fixed32"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "fixed64"
	}
	return typeName(f)
}

// typeName names the field's type the way the .proto file writes it: the
// scalar keyword, bytes or int64, or the last element of a named type,
// Link for .app.record.Link.
func typeName(f *descriptorpb.FieldDescriptorProto) string {
	if n := f.GetTypeName(); n != "" {
		return n[strings.LastIndex(n, ".")+1:]
	}
	return strings.ToLower(strings.TrimPrefix(f.GetType().String(), "TYPE_"))
}
