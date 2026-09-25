package pb

import (
	"os"
	"path/filepath"
	"slices"

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
// keeps its number, a number a field of the committed message held cannot
// pass to a field of another name, and neither can a number or a name the
// committed message reserves be taken; each breach is reported with the
// file to delete for accepting the break. The numbers and names of the
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
		reservedNumbers := make(map[int32]bool)
		for _, r := range committed.GetReservedRange() {
			for n := r.GetStart(); n < r.GetEnd(); n++ {
				reservedNumbers[n] = true
			}
		}
		reservedNames := make(map[string]bool, len(committed.GetReservedName()))
		for _, n := range committed.GetReservedName() {
			reservedNames[n] = true
		}
		current := make(map[string]bool, len(msg.GetField()))
		for _, f := range msg.GetField() {
			current[f.GetName()] = true
			switch holder, held := numbers[f.GetNumber()]; {
			case names[f.GetName()] != 0 && names[f.GetName()] != f.GetNumber():
				g.project.Report(s, "the field %s of message %s was number %d and is now %d; keep %d, or delete %s to accept the break", f.GetName(), name, names[f.GetName()], f.GetNumber(), names[f.GetName()], s.Subject)
			case held && holder != f.GetName():
				g.project.Report(s, "the field %s of message %s takes number %d, which the field %s held; a number is never reused, so give %s a fresh number and let %d stay reserved, or delete %s to accept the break", f.GetName(), name, f.GetNumber(), holder, f.GetName(), f.GetNumber(), s.Subject)
			case reservedNumbers[f.GetNumber()]:
				g.project.Report(s, "the field %s of message %s takes number %d, which the file reserves; give %s a fresh number, or delete %s to accept the break", f.GetName(), name, f.GetNumber(), f.GetName(), s.Subject)
			case reservedNames[f.GetName()]:
				g.project.Report(s, "the field %s of message %s takes a name the file reserves; a removed field's name is never reused, so name it differently, or delete %s to accept the break", f.GetName(), name, s.Subject)
			}
		}
		for _, f := range committed.GetField() {
			if !current[f.GetName()] {
				reservedNumbers[f.GetNumber()] = true
				reservedNames[f.GetName()] = true
			}
		}
		msg.ReservedRange = reservedRanges(reservedNumbers)
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

// reservedRanges turns a set of numbers into the ranges a message reserves,
// sorted, consecutive numbers joined into one: {12, 13, 20} gives 12 to 13
// and 20, the end of each range being exclusive as the descriptor has it.
func reservedRanges(numbers map[int32]bool) []*descriptorpb.DescriptorProto_ReservedRange {
	sorted := make([]int32, 0, len(numbers))
	for n := range numbers {
		sorted = append(sorted, n)
	}
	slices.Sort(sorted)
	var ranges []*descriptorpb.DescriptorProto_ReservedRange
	for _, n := range sorted {
		if last := len(ranges) - 1; last >= 0 && ranges[last].GetEnd() == n {
			ranges[last].End = new(n + 1)
			continue
		}
		ranges = append(ranges, &descriptorpb.DescriptorProto_ReservedRange{Start: new(n), End: new(n + 1)})
	}
	return ranges
}
