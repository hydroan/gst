package pb

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestGoCamelCaseNamesLikeTheProtobufPlugin pins the examples of the
// goCamelCase doc comment.
func TestGoCamelCaseNamesLikeTheProtobufPlugin(t *testing.T) {
	for name, want := range map[string]string{
		"sort_by":       "SortBy",
		"box_id":        "BoxId",
		"x_id":          "XId",
		"foo_2bar":      "Foo_2Bar",
		"_summary":      "XSummary",
		"Record":        "Record",
		"Record.Window": "Record_Window",
	} {
		require.Equal(t, want, goCamelCase(name), name)
	}
}

// TestGoFieldNamesMatchWhatTheProtobufPluginWrites holds goFieldNames and
// goCamelCase to the plugin itself: a message carrying every naming case the
// doc comments describe is compiled with Compile, and the struct the plugin
// writes declares exactly the field names the two derive. The field names
// are read off the struct's own lines, one field per line, so a rule
// drifting from the plugin's fails here rather than in a project that no
// longer builds.
func TestGoFieldNamesMatchWhatTheProtobufPluginWrites(t *testing.T) {
	desc := newMessage(
		stringField("title", 1),
		stringField("reset", 2),
		stringField("get_title", 3),
		stringField("sort_by", 4),
		stringField("x_id", 5),
		stringField("foo_2bar", 6),
		stringField("descriptor", 7),
		stringField("summary", 8),
		stringField("x_summary", 9),
	)
	desc.Name = new("Naming")
	desc.Field[7].Proto3Optional = new(true)
	desc.Field[7].OneofIndex = new(int32(0))
	desc.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: new("_summary")}}

	want := map[string]string{
		"title":      "Title",
		"reset":      "Reset_",
		"get_title":  "GetTitle_",
		"sort_by":    "SortBy",
		"x_id":       "XId",
		"foo_2bar":   "Foo_2Bar",
		"descriptor": "Descriptor_",
		"summary":    "Summary",
		"x_summary":  "XSummary_",
	}
	require.Equal(t, want, goFieldNames(desc))

	compiled, err := Compile([]File{{Path: "pb/naming.proto", Content: `syntax = "proto3";

package tmpapp;

option go_package = "tmpapp/pb;pb";

message Naming {
  string title = 1;
  string reset = 2;
  string get_title = 3;
  string sort_by = 4;
  string x_id = 5;
  string foo_2bar = 6;
  string descriptor = 7;
  optional string summary = 8;
  string x_summary = 9;
}
`}})
	require.NoError(t, err)
	require.Len(t, compiled, 1)
	source := compiled[0].Content
	structStart := strings.Index(source, "type Naming struct {")
	require.NotEqual(t, -1, structStart)
	structSource := source[structStart:]
	structEnd := strings.Index(structSource, "\n}")
	require.NotEqual(t, -1, structEnd)
	structSource = structSource[:structEnd]
	fieldLine := regexp.MustCompile(`(?m)^\t(\w+) +(\*?string) +` + "`" + `protobuf:"bytes,\d+,opt,name=(\w+)`)
	written := make(map[string]string)
	for _, match := range fieldLine.FindAllStringSubmatch(structSource, -1) {
		written[match[3]] = match[1]
	}
	require.Equal(t, want, written, "the plugin wrote:\n%s", structSource)
}
