package pb

import (
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file holds the Go names protoc-gen-go gives the messages and the
// fields of the definitions, which the generated handlers refer to. The
// rules are the plugin's own, spelled in google.golang.org/protobuf's
// internal/strs and compiler/protogen packages; the first cannot be imported
// at all and the second names a field only while running as the plugin, so
// they are restated here and a test holds them to what the plugin writes.

// goCamelCase is the Go identifier protoc-gen-go makes of a protobuf name:
// every word, marked by an underscore or an upper case letter, starts upper
// case, an underscore before a lower case letter goes and any other stays, a
// leading underscore becomes X, and a dot, which joins a nested message to
// its parent, becomes an underscore. So sort_by gives SortBy, box_id BoxId,
// x_id XId, foo_2bar Foo_2Bar, _summary XSummary and Record.Window
// Record_Window, while Record stays as it is.
func goCamelCase(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.' && i+1 < len(s) && isASCIILower(s[i+1]):
			// A dot before a lower case letter goes, the letter starting
			// the next word.
		case c == '.':
			b = append(b, '_')
		case c == '_' && (i == 0 || s[i-1] == '.'):
			// A leading underscore, of a name or of a nested one, becomes
			// X so that the identifier starts with a capital letter.
			b = append(b, 'X')
		case c == '_' && i+1 < len(s) && isASCIILower(s[i+1]):
			// An underscore before a lower case letter goes, the letter
			// starting the next word.
		case isASCIIDigit(c):
			b = append(b, c)
		default:
			// A letter starts a word: it goes upper case, and the lower
			// case letters after it follow as they are.
			if isASCIILower(c) {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			for ; i+1 < len(s) && isASCIILower(s[i+1]); i++ {
				b = append(b, s[i+1])
			}
		}
	}
	return string(b)
}

func isASCIILower(c byte) bool { return 'a' <= c && c <= 'z' }
func isASCIIDigit(c byte) bool { return '0' <= c && c <= '9' }

// messageMethods are the methods every generated message has, which no
// field may be named like.
var messageMethods = []string{"Reset", "String", "ProtoMessage", "Marshal", "Unmarshal", "ExtensionRangeArray", "ExtensionMap", "Descriptor"}

// goFieldNames returns the Go name of every field of desc, keyed by the
// field's protobuf name, the way protoc-gen-go names them in order: the camel
// case of the name (see goCamelCase), given a trailing underscore for as long
// as it is a method every message has or the getter of a field named before
// it. So title gives Title, reset Reset_, and get_title after title
// GetTitle_, its getter being GetGetTitle_. The synthetic oneof of a proto3
// optional field takes its name after the field, as the plugin does: an
// optional summary followed by x_summary gives XSummary_ for the latter.
func goFieldNames(desc *descriptorpb.DescriptorProto) map[string]string {
	used := make(map[string]bool, len(messageMethods)+3*len(desc.GetField()))
	for _, method := range messageMethods {
		used[method] = true
	}
	unique := func(name string, getter bool) string {
		for used[name] || (getter && used["Get"+name]) {
			name += "_"
		}
		used[name] = true
		used["Get"+name] = getter
		return name
	}
	names := make(map[string]string, len(desc.GetField()))
	for _, field := range desc.GetField() {
		names[field.GetName()] = unique(goCamelCase(field.GetName()), true)
		if field.GetProto3Optional() {
			unique(goCamelCase(desc.GetOneofDecl()[field.GetOneofIndex()].GetName()), false)
		}
	}
	return names
}
