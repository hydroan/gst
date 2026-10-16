package pb

import "google.golang.org/protobuf/types/descriptorpb"

// stringField returns a singular string field, for the descriptors the tests
// of this package build by hand.
func stringField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   new(name),
		Number: new(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
	}
}

// messageField returns a singular field of the message typeName names.
func messageField(name string, number int32, typeName string) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     new(name),
		Number:   new(number),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: new(typeName),
	}
}

// repeatedMessageField returns a repeated field of the message typeName
// names, numbered 0 as a field is before numbering.
func repeatedMessageField(name, typeName string) *descriptorpb.FieldDescriptorProto {
	field := messageField(name, 0, typeName)
	field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return field
}
