package pb

import (
	"strings"
	"testing"

	"github.com/jhump/protoreflect/v2/protoprint"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestReadBackReportsATypeThePrintedFileResolvesElsewhere pins readBack on
// the file the generator once printed for a model named Item with a
// PatchMany action: the request nested a message named Item whose field
// referred to the top-level Item by its relative name, which the printed
// file resolves to the nested message itself. A file whose every reference
// resolves as the descriptor means passes.
func TestReadBackReportsATypeThePrintedFileResolvesElsewhere(t *testing.T) {
	item := &descriptorpb.DescriptorProto{Name: new("Item"), Field: []*descriptorpb.FieldDescriptorProto{stringField("content", 1)}}
	nested := &descriptorpb.DescriptorProto{Name: new("Item"), Field: []*descriptorpb.FieldDescriptorProto{messageField("item", 1, ".tmpapp.Item")}}
	request := &descriptorpb.DescriptorProto{
		Name:       new("PatchManyItemRequest"),
		Field:      []*descriptorpb.FieldDescriptorProto{repeatedMessageField("items", ".tmpapp.PatchManyItemRequest.Item")},
		NestedType: []*descriptorpb.DescriptorProto{nested},
	}
	request.Field[0].Number = new(int32(1))
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        new("item.proto"),
		Package:     new("tmpapp"),
		Syntax:      new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{item, request},
	}, nil)
	require.NoError(t, err)
	var printed strings.Builder
	printer := protoprint.Printer{}
	require.NoError(t, printer.PrintProtoFile(fd, &printed))
	require.Contains(t, printed.String(), "Item item = 1;", "the relative name the printer writes is the shadowed one")

	err = readBack([]File{{Path: "pb/item.proto", Content: printed.String()}}, map[string]protoreflect.FileDescriptor{"item.proto": fd})

	require.EqualError(t, err, "pb/item.proto: the field tmpapp.PatchManyItemRequest.Item.item reads as tmpapp.PatchManyItemRequest.Item, the message nested in tmpapp.PatchManyItemRequest, where the definition means tmpapp.Item: the nested message shadows it; name the field or the type differently")

	plain, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        new("plain.proto"),
		Package:     new("tmpapp"),
		Syntax:      new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{item},
	}, nil)
	require.NoError(t, err)
	printed.Reset()
	require.NoError(t, printer.PrintProtoFile(plain, &printed))
	require.NoError(t, readBack([]File{{Path: "pb/plain.proto", Content: printed.String()}}, map[string]protoreflect.FileDescriptor{"plain.proto": plain}))
}
