package pb

import (
	"go/types"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/codegen/gen/jsonshape"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file builds the service of a model: an rpc per action gRPC can serve,
// with the request and response messages of the standard actions and the Go
// types of the custom ones.

// httpOnlyPhases are the actions gRPC does not serve: Import reads a
// multipart upload, Export answers with a file attachment and SSE is an HTTP
// protocol of its own.
var httpOnlyPhases = map[consts.Phase]bool{
	consts.PHASE_IMPORT: true,
	consts.PHASE_EXPORT: true,
	consts.PHASE_SSE:    true,
}

// declareService builds the service of m in the file mirroring its model
// file: <Model>Service with an rpc per action of every route (see rpcName),
// each taking and returning the messages rpcMessages resolves. The model's
// own message is queued unless the model is virtual. Two actions resolving to
// one rpc name are reported.
func (g *generator) declareService(m *gen.ModelInfo) {
	s := jsonshape.Site{Subject: m.ImportPath() + "." + m.ModelName}
	pkg := g.project.Package(m.ImportPath())
	if pkg == nil || pkg.Types == nil {
		g.project.Report(s, "the package is not part of module %s", g.cfg.ModulePath)
		return
	}
	obj, ok := pkg.Types.Scope().Lookup(m.ModelName).(*types.TypeName)
	if !ok {
		g.project.Report(s, "the package declares no type %s", m.ModelName)
		return
	}
	s.Pos = obj.Pos()
	file := g.fileOf(obj)
	var model *message
	if !m.Design.IsEmpty {
		// The model's message and the types it reaches come first in the
		// file, before the messages of its actions.
		model = g.messageOf(obj)
		g.buildQueued()
	}

	service := &descriptorpb.ServiceDescriptorProto{Name: new(m.ModelName + "Service")}
	if holder, ok := file.claim(service.GetName(), "the service of "+m.ModelName); !ok {
		g.project.Report(s, "the service %s clashes with %s; rename the type", service.GetName(), holder)
		return
	}
	routes := make(map[string]string)
	m.Design.Range(func(route string, action *dsl.Action) {
		if httpOnlyPhases[action.Phase] {
			return
		}
		name := rpcName(m, route, action)
		if previous, taken := routes[name]; taken {
			g.project.Report(s, "the %s actions on routes %s and %s both become rpc %s; name one of them with Filename()", action.Phase.MethodName(), previous, route, name)
			return
		}
		routes[name] = route
		input, output, ok := g.rpcMessages(m, pkg.Types.Scope(), model, file, route, action, s)
		if !ok {
			return
		}
		file.comment([]int32{fileServicesTag, int32Index(len(file.services)), serviceMethodsTag, int32Index(len(service.Method))},
			name+" is the "+action.Phase.MethodName()+" action of "+m.ModelName+" on "+route+".")
		service.Method = append(service.Method, &descriptorpb.MethodDescriptorProto{
			Name:       new(name),
			InputType:  new(input),
			OutputType: new(output),
		})
	})
	file.addService(service, service.GetName()+" serves the actions of "+m.ModelName+" over gRPC.")
}

// rpcMessages resolves the request and response messages of an action and
// returns their fully-qualified names. A standard action, one whose request
// and response are the model itself, gets the messages standardMessages
// builds; a custom action uses the Go types its Payload and Result declare,
// and an empty message named like a standard one for the side it leaves
// undeclared.
func (g *generator) rpcMessages(m *gen.ModelInfo, scope *types.Scope, model *message, file *protoFile, route string, action *dsl.Action, s jsonshape.Site) (string, string, bool) {
	self := "*" + m.ModelName
	if action.Payload == self && action.Result == self {
		if model == nil {
			g.project.Report(s, "the %s action of the virtual model %s has no message to carry; declare Payload and Result", action.Phase.MethodName(), m.ModelName)
			return "", "", false
		}
		return g.standardMessages(m, model, file, route, action)
	}
	input, ok := g.customMessage(m, scope, file, route, action, action.Payload, "Request", s)
	if !ok {
		return "", "", false
	}
	output, ok := g.customMessage(m, scope, file, route, action, action.Result, "Response", s)
	if !ok {
		return "", "", false
	}
	return input, output, true
}

// customMessage resolves one side of a custom action: the message of the Go
// type named, queued to be built, or an empty message named like a standard
// one (see standardMessageName) when the side is undeclared.
func (g *generator) customMessage(m *gen.ModelInfo, scope *types.Scope, file *protoFile, route string, action *dsl.Action, typeName, kind string, s jsonshape.Site) (string, bool) {
	if typeName == "" || typeName == dsl.PayloadEmpty {
		name := standardMessageName(m, route, action, kind)
		if holder, ok := file.claim(name, "the rpc "+m.ModelName+"Service."+rpcName(m, route, action)); !ok {
			g.project.Report(s, "the message %s clashes with %s; rename the type", name, holder)
			return "", false
		}
		file.addMessage(&descriptorpb.DescriptorProto{Name: new(name)},
			name+" is the empty "+strings.ToLower(kind)+" of "+m.ModelName+"Service."+rpcName(m, route, action)+".")
		return "." + file.pkg + "." + name, true
	}
	obj, ok := scope.Lookup(strings.TrimPrefix(typeName, "*")).(*types.TypeName)
	if !ok {
		g.project.Report(s, "the %s action declares the type %s, which the model's package does not declare", action.Phase.MethodName(), typeName)
		return "", false
	}
	// An alias stands for another type, whose message it shares.
	if obj.IsAlias() {
		named, isNamed := types.Unalias(obj.Type()).(*types.Named)
		if !isNamed {
			g.project.Report(s, "the %s action declares %s, an alias of %s, which is not a named type; declare a struct type", action.Phase.MethodName(), typeName, types.Unalias(obj.Type()))
			return "", false
		}
		obj = named.Obj()
	}
	msg := g.messageOf(obj)
	file.importOf(msg.file.name)
	return msg.fullName(), true
}

// standardMessages builds the request and response messages of a standard
// action in file and returns their fully-qualified names (see
// standardMessageName). With X the model and x its field name
// (see modelFieldName), the messages are:
//
//	Create:     CreateXRequest { X x = 1; }                                             CreateXResponse { X x = 1; }
//	Get:        GetXRequest { string id = 1; repeated string expand = 2; uint32 depth = 3; }   GetXResponse { X x = 1; }
//	Update:     UpdateXRequest { string id = 1; X x = 2; }                              UpdateXResponse { X x = 1; }
//	Patch:      PatchXRequest { string id = 1; X x = 2; FieldMask update_mask = 3; }    PatchXResponse { X x = 1; }
//	Delete:     DeleteXRequest { string id = 1; }                                        DeleteXResponse {}
//	List:       ListXRequest { repeated Filter filters = 1; repeated string sort_by = 2; uint32 page = 3; uint32 size = 4;
//	                           string cursor_field = 5; string cursor_value = 6; bool cursor_next = 7;
//	                           repeated string expand = 8; uint32 depth = 9; }         ListXResponse { repeated X items = 1; int64 total = 2; }
//	            with the nested Filter { string field = 1; string op = 2; repeated string values = 3; }
//	CreateMany, UpdateMany:
//	            ...XRequest { repeated X items = 1; }                                   ...XResponse { repeated X items = 1; }
//	PatchMany:  PatchManyXRequest { repeated Item items = 1; }                          PatchManyXResponse { repeated X items = 1; }
//	            with the nested Item { X x = 1; FieldMask update_mask = 2; }
//	DeleteMany: DeleteManyXRequest { repeated string ids = 1; }                         DeleteManyXResponse {}
func (g *generator) standardMessages(m *gen.ModelInfo, model *message, file *protoFile, route string, action *dsl.Action) (string, string, bool) {
	x := modelFieldName(m)
	rpc := m.ModelName + "Service." + rpcName(m, route, action)
	var request, response *descriptorpb.DescriptorProto
	requestFields := make([]string, 0, 4)
	responseFields := make([]string, 0, 2)
	switch action.Phase {
	case consts.PHASE_CREATE:
		request = newMessage(modelField(x, 1, model))
		requestFields = append(requestFields, "the "+m.ModelName+" to create")
		response = newMessage(modelField(x, 1, model))
		responseFields = append(responseFields, "the "+m.ModelName+" created")
	case consts.PHASE_GET:
		request = newMessage(stringField("id", 1), repeatedStringField("expand", 2), scalarField("depth", 3, descriptorpb.FieldDescriptorProto_TYPE_UINT32))
		requestFields = append(requestFields, "the id of the "+m.ModelName, "the associations to expand, as the _expand query parameter names them", "the depth of the expansion, as the _depth query parameter")
		response = newMessage(modelField(x, 1, model))
		responseFields = append(responseFields, "the "+m.ModelName+" found")
	case consts.PHASE_UPDATE:
		request = newMessage(stringField("id", 1), modelField(x, 2, model))
		requestFields = append(requestFields, "the id of the "+m.ModelName+" to replace", "the replacement")
		response = newMessage(modelField(x, 1, model))
		responseFields = append(responseFields, "the "+m.ModelName+" as stored")
	case consts.PHASE_PATCH:
		file.importOf(fieldMaskProto)
		request = newMessage(stringField("id", 1), modelField(x, 2, model), messageField("update_mask", 3, wellKnownFieldMask))
		requestFields = append(requestFields, "the id of the "+m.ModelName+" to patch", "the values to apply", "the fields of "+x+" to apply, named as the message names them")
		response = newMessage(modelField(x, 1, model))
		responseFields = append(responseFields, "the "+m.ModelName+" as stored")
	case consts.PHASE_DELETE:
		request = newMessage(stringField("id", 1))
		requestFields = append(requestFields, "the id of the "+m.ModelName+" to delete")
		response = newMessage()
	case consts.PHASE_LIST:
		filter := newMessage(stringField("field", 1), stringField("op", 2), repeatedStringField("values", 3))
		filter.Name = new("Filter")
		request = newMessage(
			repeatedMessageField("filters", "Filter"),
			repeatedStringField("sort_by", 2),
			scalarField("page", 3, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			scalarField("size", 4, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			stringField("cursor_field", 5),
			stringField("cursor_value", 6),
			scalarField("cursor_next", 7, descriptorpb.FieldDescriptorProto_TYPE_BOOL),
			repeatedStringField("expand", 8),
			scalarField("depth", 9, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
		)
		request.NestedType = append(request.NestedType, filter)
		requestFields = append(requestFields,
			"the filters to apply, each one field[op]=value of the HTTP query",
			"the orderings, as the _sort_by query parameter names them",
			"the page to list, as the _page query parameter",
			"the page size, as the _size query parameter",
			"the cursor column, as the _cursor_field query parameter",
			"the cursor position, as the _cursor_value query parameter",
			"whether to list past the cursor, as the _cursor_next query parameter",
			"the associations to expand, as the _expand query parameter names them",
			"the depth of the expansion, as the _depth query parameter")
		response = newMessage(repeatedMessageField("items", model.fullName()), scalarField("total", 2, descriptorpb.FieldDescriptorProto_TYPE_INT64))
		responseFields = append(responseFields, "the "+m.ModelName+" records of the page", "the number of records the filters match, 0 under cursor pagination")
	case consts.PHASE_CREATE_MANY, consts.PHASE_UPDATE_MANY:
		request = newMessage(repeatedMessageField("items", model.fullName()))
		requestFields = append(requestFields, "the "+m.ModelName+" records to write")
		response = newMessage(repeatedMessageField("items", model.fullName()))
		responseFields = append(responseFields, "the "+m.ModelName+" records as stored")
	case consts.PHASE_PATCH_MANY:
		file.importOf(fieldMaskProto)
		item := newMessage(modelField(x, 1, model), messageField("update_mask", 2, wellKnownFieldMask))
		item.Name = new("Item")
		request = newMessage(repeatedMessageField("items", "Item"))
		request.NestedType = append(request.NestedType, item)
		requestFields = append(requestFields, "the patches, each naming the "+m.ModelName+" it applies to by its id")
		response = newMessage(repeatedMessageField("items", model.fullName()))
		responseFields = append(responseFields, "the "+m.ModelName+" records as stored")
	case consts.PHASE_DELETE_MANY:
		request = newMessage(repeatedStringField("ids", 1))
		requestFields = append(requestFields, "the ids of the "+m.ModelName+" records to delete")
		response = newMessage()
	}
	requestName := standardMessageName(m, route, action, "Request")
	responseName := standardMessageName(m, route, action, "Response")
	request.Name = new(requestName)
	response.Name = new(responseName)
	for _, name := range []string{requestName, responseName} {
		if holder, ok := file.claim(name, "the rpc "+rpc); !ok {
			g.project.Report(jsonshape.Site{Subject: m.ImportPath() + "." + m.ModelName}, "the message %s clashes with %s; rename the type", name, holder)
			return "", "", false
		}
	}
	g.addStandardMessage(file, request, requestName+" is the request of "+rpc+".", requestFields)
	g.addStandardMessage(file, response, responseName+" is the response of "+rpc+".", responseFields)
	return "." + file.pkg + "." + requestName, "." + file.pkg + "." + responseName, true
}

// addStandardMessage appends a standard message to file, with the comment of
// the message and one per field, each field's comment starting with its name.
func (g *generator) addStandardMessage(file *protoFile, desc *descriptorpb.DescriptorProto, comment string, fieldComments []string) {
	index := int32Index(len(file.messages))
	for i, text := range fieldComments {
		file.comment([]int32{fileMessagesTag, index, messageFieldsTag, int32Index(i)}, desc.Field[i].GetName()+" is "+text+".")
	}
	file.addMessage(desc, comment)
}

// newMessage returns a message of the given fields, to be named by the caller.
func newMessage(fields ...*descriptorpb.FieldDescriptorProto) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{Field: fields}
}

// scalarField returns a singular field of a scalar type.
func scalarField(name string, number int32, kind descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   new(name),
		Number: new(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   kind.Enum(),
	}
}

// stringField returns a singular string field.
func stringField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return scalarField(name, number, descriptorpb.FieldDescriptorProto_TYPE_STRING)
}

// repeatedStringField returns a repeated string field.
func repeatedStringField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	field := stringField(name, number)
	field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return field
}

// messageField returns a singular field of the message typeName names.
func messageField(name string, number int32, typeName string) *descriptorpb.FieldDescriptorProto {
	field := scalarField(name, number, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE)
	field.TypeName = new(typeName)
	return field
}

// repeatedMessageField returns a repeated field of the message typeName
// names, numbered 1: it is the first field of every standard message holding
// one.
func repeatedMessageField(name string, typeName string) *descriptorpb.FieldDescriptorProto {
	field := messageField(name, 1, typeName)
	field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return field
}

// modelField returns the singular field carrying the model's message.
func modelField(name string, number int32, model *message) *descriptorpb.FieldDescriptorProto {
	return messageField(name, number, model.fullName())
}
