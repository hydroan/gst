package pb

import (
	"maps"
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
// number changes hands, the fields the models dropped stay reserved, and
// every message, service and rpc is still declared, each rpc with the
// messages and the streaming it had (after Buf's breaking rules
// MESSAGE_NO_DELETE, SERVICE_NO_DELETE, RPC_NO_DELETE, RPC_SAME_REQUEST_TYPE,
// RPC_SAME_RESPONSE_TYPE, RPC_SAME_CLIENT_STREAMING and
// RPC_SAME_SERVER_STREAMING).

// reconcile reads, for every file about to be written, the committed one
// under pb/ in the project and holds the new file to it, message by message
// (see holdMessage) and service by service (see holdServices), the messages
// the committed file declared having to be declared still. A committed file
// that cannot be parsed is reported, so that it is never overwritten
// blindly; a file without a committed counterpart is new and free.
func (g *generator) reconcile() {
	for name, f := range g.files {
		s := jsonshape.Site{Subject: ggconst.DirPB + "/" + name}
		committed := g.committed(name)
		switch {
		case committed.absent:
			continue
		case committed.err != nil:
			g.project.Report(s, "the committed file cannot be read: %v; fix it or delete it", committed.err)
			continue
		}
		for _, m := range f.messages {
			g.holdMessage(s, m, "", committed.messages)
		}
		declared := make(map[string]*descriptorpb.DescriptorProto, len(committed.messages))
		indexMessages(f.messages, "", declared)
		for _, name := range slices.Sorted(maps.Keys(committed.messages)) {
			// The entry message of a map field goes with the field, whose
			// number holdMessage reserves: no client was built against it.
			if _, ok := declared[name]; !ok && !committed.messages[name].GetOptions().GetMapEntry() {
				g.project.Report(s, "the message %s is gone; a client was built against it, so keep it, or remove it from %s to accept the break", name, s.Subject)
			}
		}
		g.holdServices(s, f, committed.services)
	}
}

// committedFile is a definition already under pb/ as reconcile and the
// numbering of untagged fields read it: absent when the project holds none,
// otherwise its messages by dotted name (see indexMessages) and its services
// by name, or what kept it from being read.
type committedFile struct {
	absent   bool
	messages map[string]*descriptorpb.DescriptorProto
	services map[string]*descriptorpb.ServiceDescriptorProto
	err      error
}

// committed returns the committed counterpart of the file name under pb/,
// read once.
func (g *generator) committed(name string) committedFile {
	if c, ok := g.committedFiles[name]; ok {
		return c
	}
	var c committedFile
	path := filepath.Join(g.cfg.Dir, ggconst.DirPB, filepath.FromSlash(name))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.absent = true
	} else if desc, err := readCommitted(path); err != nil {
		c.err = err
	} else {
		c.messages = make(map[string]*descriptorpb.DescriptorProto)
		indexMessages(desc.GetMessageType(), "", c.messages)
		c.services = make(map[string]*descriptorpb.ServiceDescriptorProto, len(desc.GetService()))
		for _, svc := range desc.GetService() {
			c.services[svc.GetName()] = svc
		}
	}
	g.committedFiles[name] = c
	return c
}

// committedMessage returns the message protoName of the committed
// counterpart of file, nil when the project holds no such file, or no such
// message, or the file cannot be read, which reconcile reports.
func (g *generator) committedMessage(file *protoFile, protoName string) *descriptorpb.DescriptorProto {
	return g.committed(file.name).messages[protoName]
}

// readCommitted parses the .proto file at path on its own, imports
// unresolved, which is all the comparison needs: the messages, their fields
// with numbers, and what they reserve. The parser is bufbuild/protocompile's,
// the front end of a protobuf compiler in pure Go, so no protoc is needed to
// read the file.
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
// messages, by its dotted name below the file: Record, and Record.Window
// for a message a committed file nests in Record.
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
// message reserves be taken; each breach is reported with the edit of the
// committed file that accepts the break, which keeps what the file reserves
// where deleting it would not. The numbers and names of the
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
				g.project.Report(s, "the field %s of message %s was number %d and is now %d; keep %d, or replace the field in %s by \"reserved %d;\" to accept the break", f.GetName(), name, was.GetNumber(), f.GetNumber(), was.GetNumber(), s.Subject, was.GetNumber())
			case known && repeated(was) != repeated(f):
				g.project.Report(s, "the field %s of message %s was %s and is now %s; a cardinality change breaks the wire, so keep it %s, or remove the field from %s to accept the break", f.GetName(), name, cardinality(was), cardinality(f), cardinality(was), s.Subject)
			case known && wireType(was) != wireType(f):
				g.project.Report(s, "the field %s of message %s was %s and is now %s; a type change breaks the wire, so keep %s or a type compatible with it, or remove the field from %s to accept the break", f.GetName(), name, typeName(was), typeName(f), typeName(was), s.Subject)
			case held && holder != f.GetName():
				g.project.Report(s, "the field %s of message %s takes number %d, which the field %s held; a number is never reused, so give %s a fresh number and let %d stay reserved, or remove the field %s from %s to accept the break", f.GetName(), name, f.GetNumber(), holder, f.GetName(), f.GetNumber(), holder, s.Subject)
			case reserves(reserved, f.GetNumber()):
				g.project.Report(s, "the field %s of message %s takes number %d, which the file reserves; give %s a fresh number, or remove the reservation of %d from %s to accept the break", f.GetName(), name, f.GetNumber(), f.GetName(), f.GetNumber(), s.Subject)
			case reservedNames[f.GetName()]:
				g.project.Report(s, "the field %s of message %s takes a name the file reserves; a removed field's name is never reused, so name it differently, or remove the reservation of the name from %s to accept the break", f.GetName(), name, s.Subject)
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

// holdServices holds the services of file to the committed ones, old: a
// committed service must still be declared, and so must every rpc of it,
// taking the request message, answering the response message and streaming
// the sides it did, since a client was built against each; every breach is
// reported with the edit of the committed file that accepts the break. A committed
// NoteService whose GetNote took GetNoteRequest and answered GetNoteResponse
// as a unary rpc holds the file to exactly that.
func (g *generator) holdServices(s jsonshape.Site, file *protoFile, old map[string]*descriptorpb.ServiceDescriptorProto) {
	services := make(map[string]*descriptorpb.ServiceDescriptorProto, len(file.services))
	for _, svc := range file.services {
		services[svc.GetName()] = svc
	}
	for _, name := range slices.Sorted(maps.Keys(old)) {
		svc, ok := services[name]
		if !ok {
			g.project.Report(s, "the service %s is gone; a client was built against it, so keep it, or remove it from %s to accept the break", name, s.Subject)
			continue
		}
		methods := make(map[string]*descriptorpb.MethodDescriptorProto, len(svc.GetMethod()))
		for _, m := range svc.GetMethod() {
			methods[m.GetName()] = m
		}
		for _, was := range old[name].GetMethod() {
			m, ok := methods[was.GetName()]
			switch {
			case !ok:
				g.project.Report(s, "the rpc %s of service %s is gone; a client was built against it, so keep it, or remove it from %s to accept the break", was.GetName(), name, s.Subject)
			case messageOfType(was.GetInputType()) != messageOfType(m.GetInputType()):
				g.project.Report(s, "the rpc %s of service %s took %s and now takes %s; a client was built against it, so keep it, or remove the rpc from %s to accept the break", was.GetName(), name, messageOfType(was.GetInputType()), messageOfType(m.GetInputType()), s.Subject)
			case messageOfType(was.GetOutputType()) != messageOfType(m.GetOutputType()):
				g.project.Report(s, "the rpc %s of service %s answered %s and now answers %s; a client was built against it, so keep it, or remove the rpc from %s to accept the break", was.GetName(), name, messageOfType(was.GetOutputType()), messageOfType(m.GetOutputType()), s.Subject)
			case was.GetClientStreaming() != m.GetClientStreaming() || was.GetServerStreaming() != m.GetServerStreaming():
				g.project.Report(s, "the rpc %s of service %s was %s and is now %s; a change of streaming breaks the wire, so keep it %s, or remove the rpc from %s to accept the break", was.GetName(), name, streaming(was), streaming(m), streaming(was), s.Subject)
			}
		}
	}
}

// messageOfType is the name of the message an rpc's type names, below its
// package: GetNoteRequest for .app.GetNoteRequest, which a parsed file
// spells, and for GetNoteRequest as is.
func messageOfType(typeName string) string {
	return typeName[strings.LastIndex(typeName, ".")+1:]
}

// streaming names the streaming of an rpc for a diagnostic: unary, client
// streaming, server streaming or bidirectional streaming.
func streaming(m *descriptorpb.MethodDescriptorProto) string {
	switch {
	case m.GetClientStreaming() && m.GetServerStreaming():
		return "bidirectional streaming"
	case m.GetClientStreaming():
		return "client streaming"
	case m.GetServerStreaming():
		return "server streaming"
	}
	return "unary"
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
