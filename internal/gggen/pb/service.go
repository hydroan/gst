package pb

import (
	"cmp"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/hydroan/gst/internal/urlquery"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file builds the service of a model: an rpc per action gRPC can serve,
// each with a request and a response message of its own.

// declareService builds the service of m in the file mirroring its model
// file: <Model>Service with an rpc per action of every route (see rpcName),
// each taking and returning the messages rpcMessages resolves. The model's
// own message is queued unless the model is virtual. Two actions resolving to
// one rpc name are reported, and so is a model left with no action to serve,
// every one being disabled, ignored by gst.yaml or HTTP only: its GRPC()
// promises a service that would have no rpc.
//
// The Item model of the golden fixture, declaring Create and Get on its
// endpoint items under model/record/ and two routes, gets
//
//	// ItemService serves the actions of Item over gRPC.
//	service ItemService {
//	  // CreateItem is the Create action of Item on /api/records/:record/items.
//	  rpc CreateItem ( CreateItemRequest ) returns ( CreateItemResponse );
//
//	  // GetItem is the Get action of Item on /api/records/:record/items/:id.
//	  rpc GetItem ( GetItemRequest ) returns ( GetItemResponse );
//
//	  // SealItem is the Create action of Item on /api/items/:id/seal.
//	  rpc SealItem ( SealItemRequest ) returns ( SealItemResponse );
//
//	  // MergeItem is the Create action of Item on /api/items/merge.
//	  rpc MergeItem ( MergeItemRequest ) returns ( MergeItemResponse );
//	}
func (g *generator) declareService(m *modelinfo.Model) {
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
	// The standard PatchMany of a route lists its items by the request of
	// the route's standard Patch (see standardMessages), built for it when
	// the model declares no Patch there (see patchRequest); a Patch
	// declaring a Payload or Result of its own has a request of another
	// shape, which the items cannot be.
	batches := make(map[string]string) // route -> the qualified rpc of its standard PatchMany
	patches := make(map[string]*dsl.Action)
	m.Design.Range(func(route string, action *dsl.Action) {
		switch {
		case dsl.HTTPOnlyAction(action.Phase.Name()):
		case action.Phase == consts.PatchMany && standardAction(m, action):
			batches[route] = m.ModelName + "Service." + modelinfo.RPCName(m, route, action)
		case action.Phase == consts.Patch:
			patches[route] = action
		}
	})

	routes := make(map[string]string)
	served := 0
	m.Design.Range(func(route string, action *dsl.Action) {
		if dsl.HTTPOnlyAction(action.Phase.Name()) {
			return
		}
		served++
		name := modelinfo.RPCName(m, route, action)
		if previous, taken := routes[name]; taken {
			g.project.Report(s, "the %s actions on routes %s and %s both become rpc %s; name one of them with Service(\"name\")", action.Phase.Name(), previous, route, name)
			return
		}
		routes[name] = route
		itemOf := ""
		if action.Phase == consts.Patch && standardAction(m, action) {
			itemOf = batches[route]
		}
		r, ok := g.rpcMessages(m, pkg.Types.Scope(), model, file, route, action, itemOf, s)
		if !ok {
			return
		}
		// A Stream action is served over gRPC alone: its route names it and
		// tells it from the other actions, but is no path a request reaches.
		comment := name + " is the " + action.Phase.Name() + " action of " + m.ModelName + " on " + consts.APIPath(r.registered) + "."
		if dsl.GRPCOnlyAction(action.Phase.Name()) {
			comment = name + " is the " + action.Phase.Name() + " action of " + m.ModelName + " declared on " + route + ", served over gRPC alone."
		}
		file.comment([]int32{fileServicesTag, int32Index(len(file.services)), serviceMethodsTag, int32Index(len(service.Method))}, comment)
		method := &descriptorpb.MethodDescriptorProto{
			Name:       new(name),
			InputType:  new("." + file.pkg + "." + r.request.GetName()),
			OutputType: new("." + file.pkg + "." + r.response.GetName()),
		}
		// A Stream action streams the side it declares streaming: the rpc
		// takes a stream of its request message, returns a stream of its
		// response message, or both.
		if action.StreamingPayload {
			method.ClientStreaming = new(true)
		}
		if action.StreamingResult {
			method.ServerStreaming = new(true)
		}
		service.Method = append(service.Method, method)
		file.rpcs = append(file.rpcs, r)
	})
	if served == 0 {
		g.project.Report(s, "the model declares GRPC() but none of its actions is served over gRPC, every one being ignored by gst.yaml or HTTP only; remove GRPC() or declare an action gRPC serves")
		return
	}
	for _, route := range slices.Sorted(maps.Keys(batches)) {
		patch, declared := patches[route]
		switch {
		case !declared:
			g.patchRequest(m, model, file, route, batches[route], s)
		case !standardAction(m, patch):
			g.project.Report(s, "the PatchMany action on %s lists the requests of the Patch action as its items, but the Patch action there declares a Payload or Result of its own; declare the PatchMany action with a Payload and Result of its own as well", route)
		}
	}
	file.addService(service, service.GetName()+" serves the actions of "+m.ModelName+" over gRPC.")
}

// standardAction reports whether an action of m is a standard one, taking
// and answering the model itself, run through the call of its kind
// (CreateCall and its kind); a Stream action is a custom one whatever it
// declares.
func standardAction(m *modelinfo.Model, action *dsl.Action) bool {
	self := "*" + m.ModelName
	return action.Payload == self && action.Result == self && action.Phase != consts.Stream
}

// rpcMessages builds the request and response messages of an action in file
// and returns their fully-qualified names (see messageName). The request
// opens with the parameters of the route (see requestParams), gRPC having no
// path to carry them, then holds what the action reads: for a standard
// action, one whose Payload and Result are the model itself, the fields
// standardMessages lists; for a List or Get action answering with a type of
// its own, the query fields of the standard one, since the HTTP query serves
// it the same; for any other custom action, its Payload as the field
// payload. The response of a standard action is standardMessages' one; a
// custom action's holds its Result as the field result, or nothing when the
// action declares none. A Stream action is a custom one whatever it
// declares, its Payload and Result being what each message of a streamed
// side carries. Every rpc owns its two messages: two actions sharing a Go
// type share the message that type encodes to, held by their payload or
// result fields, never a request or response, but for the items of a
// standard PatchMany, the requests of the route's Patch (see
// standardMessages): itemOf, the qualified rpc of that PatchMany, is named
// in the comment of the Patch's request. A route parameter named like a
// field of the request is reported (see addRequest).
//
// The Create action of Item on records/:record/items gets
//
//	// CreateItemRequest is the request of ItemService.CreateItem.
//	message CreateItemRequest {
//	  // record is the :record parameter of /api/records/:record/items.
//	  string record = 1;
//
//	  // item is the Item to create.
//	  Item item = 2;
//	}
//
//	// CreateItemResponse is the response of ItemService.CreateItem.
//	message CreateItemResponse {
//	  // item is the Item created.
//	  Item item = 1;
//	}
func (g *generator) rpcMessages(m *modelinfo.Model, scope *types.Scope, model *message, file *protoFile, route string, action *dsl.Action, itemOf string, s jsonshape.Site) (*rpc, bool) {
	r := &rpc{name: modelinfo.RPCName(m, route, action), service: m.ModelName + "Service", model: m, action: action, route: route}
	r.registered, r.param = modelinfo.RouterTargetForAction(route, m.Design, action)
	qualified := r.service + "." + r.name
	var request, response *descriptorpb.DescriptorProto
	var requestFields, responseFields []string
	if standardAction(m, action) {
		if model == nil {
			g.project.Report(s, "the %s action of the virtual model %s has no message to carry; declare Payload and Result", action.Phase.Name(), m.ModelName)
			return nil, false
		}
		r.standard, r.message = true, model
		request, requestFields, response, responseFields = standardMessages(m, model, file, route, action)
	} else {
		var ok bool
		if request, requestFields, r.payload, ok = g.customRequest(scope, file, action, s); !ok {
			return nil, false
		}
		if response, responseFields, r.result, ok = g.customResponse(scope, file, action, s); !ok {
			return nil, false
		}
	}

	requestName := messageName(m, route, action, "Request")
	responseName := messageName(m, route, action, "Response")
	comment := requestName + " is the request of " + qualified + "."
	if itemOf != "" {
		comment = requestName + " is the request of " + qualified + " and an item of " + itemOf + "."
	}
	params, ok := g.addRequest(m, file, route, action, request, requestFields, requestName, comment, "the rpc "+qualified, s)
	if !ok {
		return nil, false
	}
	r.params = params
	if responseFields, ok = g.numberFields(s, file, responseName, response, responseFields); !ok {
		return nil, false
	}
	response.Name = new(responseName)
	if holder, ok := file.claim(responseName, "the rpc "+qualified); !ok {
		g.project.Report(s, "the message %s clashes with %s; rename the type", responseName, holder)
		return nil, false
	}
	g.addRPCMessage(file, response, responseName+" is the response of "+qualified+".", responseFields)
	r.request, r.response = request, response
	return r, true
}

// addRequest completes the request message of an rpc, or the item message
// a standard PatchMany lists (see patchRequest), and adds it to file: the
// parameters of the route (see requestParams) go first, before what request
// holds, a parameter named like a field of the request being reported; the
// fields are numbered after the committed definition (see numberFields);
// the message takes name, claimed for owner, and comment, with the comment
// of each field. It returns the parameters.
func (g *generator) addRequest(m *modelinfo.Model, file *protoFile, route string, action *dsl.Action, request *descriptorpb.DescriptorProto, requestFields []string, name, comment, owner string, s jsonshape.Site) ([]requestParam, bool) {
	params := requestParams(m, route, action)
	fields := make([]*descriptorpb.FieldDescriptorProto, 0, len(params)+len(request.Field))
	comments := make([]string, 0, len(params)+len(requestFields))
	for _, param := range params {
		fields = append(fields, stringField(param.name, 0))
		comments = append(comments, param.comment)
	}
	request.Field = append(fields, request.Field...)
	requestFields = append(comments, requestFields...)
	registered, _ := modelinfo.RouterTargetForAction(route, m.Design, action)
	for i, param := range params {
		for _, field := range request.Field[i+1:] {
			if field.GetName() == param.name {
				g.project.Report(s, "the :%s parameter of %s clashes with the %s field of %s; rename the parameter", param.param, registered, param.name, name)
				return nil, false
			}
		}
	}
	requestFields, ok := g.numberFields(s, file, name, request, requestFields)
	if !ok {
		return nil, false
	}
	request.Name = new(name)
	if holder, ok := file.claim(name, owner); !ok {
		g.project.Report(s, "the message %s clashes with %s; rename the type", name, holder)
		return nil, false
	}
	g.addRPCMessage(file, request, comment, requestFields)
	return params, true
}

// patchAction is the standard Patch of m, taking and answering the model:
// the action the items of its standard PatchMany are the requests of.
func patchAction(m *modelinfo.Model) *dsl.Action {
	return &dsl.Action{Phase: consts.Patch, Payload: "*" + m.ModelName, Result: "*" + m.ModelName}
}

// patchRequest builds the request message of the standard Patch of m on
// route when m declares no Patch there, for the standard PatchMany of the
// route, the rpc batch, to list its items by (see standardMessages): the
// message the Patch would get, commented as the item it is.
//
// The Item model of the golden fixture, declaring PatchMany and no Patch on
// its endpoint items under records/:record, gets
//
//	// PatchItemRequest is an item of ItemService.PatchManyItem: a patch of one Item.
//	message PatchItemRequest {
//	  // record is the :record parameter of /api/records/:record/items/:id.
//	  string record = 1;
//
//	  // id is the id of the Item to patch.
//	  string id = 2;
//
//	  // item is the values to apply.
//	  Item item = 3;
//
//	  // update_mask is the fields of item to apply, named as the message names them.
//	  google.protobuf.FieldMask update_mask = 4;
//	}
func (g *generator) patchRequest(m *modelinfo.Model, model *message, file *protoFile, route, batch string, s jsonshape.Site) {
	action := patchAction(m)
	request, requestFields, _, _ := standardMessages(m, model, file, route, action)
	name := messageName(m, route, action, "Request")
	g.addRequest(m, file, route, action, request, requestFields, name, name+" is an item of "+batch+": a patch of one "+m.ModelName+".", "the items of "+batch, s)
}

// rpc is one rpc of a service as the generated handler serves it (see
// handlers.go): the action it runs, the route the action's call is built on
// and the messages it converts.
type rpc struct {
	name    string // CreateItem
	service string // ItemService
	model   *modelinfo.Model
	action  *dsl.Action
	route   string // the route the action is declared on, records/:record/items
	// registered is the route the router registers the action under,
	// records/:record/items/:id for its Get, the route the call is built on
	// and the service registry keys the service by; param is the parameter
	// that route ends in, id, which names the record an item action reads
	// (the DSL refuses an item action on a route without one).
	registered string
	param      string
	params     []requestParam
	// standard marks an action run through the call of the model,
	// CreateCall and its kind, on message, the model's; a custom action,
	// one declaring a Payload or Result of its own, runs through
	// ServiceCall on payload and result, either nil for a side not declared.
	standard        bool
	message         *message
	payload, result *message
	// request and response are the messages of the rpc.
	request, response *descriptorpb.DescriptorProto
}

// streaming reports whether the rpc streams a side of the call, which the
// rpcs of a Stream action do: streamHandler serves them, and the
// registration describes them by the stream word.
func (r *rpc) streaming() bool {
	return r.action.StreamingPayload || r.action.StreamingResult
}

// customRequest builds what the request of a custom action holds after the
// route parameters, with the comment of each field: the query fields of a
// List or Get action (see queryFields), the Payload of any other action as
// the field payload, or nothing when the action declares no Payload.
//
// The Create action of Item on items/merge, declaring Payload[*MergeReq],
// gets
//
//	// MergeItemRequest is the request of ItemService.MergeItem.
//	message MergeItemRequest {
//	  // payload is the MergeReq the action takes.
//	  MergeReq payload = 1;
//	}
//
// and the Get action of Report on reports/summary, a GET, gets
//
//	// GetReportRequest is the request of ReportService.GetReport.
//	message GetReportRequest {
//	  // expand is the associations to expand, as the _expand query parameter names them.
//	  repeated string expand = 1;
//
//	  // depth is the depth of the expansion, as the _depth query parameter.
//	  uint32 depth = 2;
//	}
func (g *generator) customRequest(scope *types.Scope, file *protoFile, action *dsl.Action, s jsonshape.Site) (*descriptorpb.DescriptorProto, []string, *message, bool) {
	if action.Phase == consts.List || action.Phase == consts.Get {
		fields, comments, nested := queryFields(action.Phase)
		request := newMessage(fields...)
		request.NestedType = nested
		return request, comments, nil, true
	}
	if action.Payload == "" || action.Payload == dsl.PayloadEmpty {
		return newMessage(), nil, nil, true
	}
	msg, ok := g.typeMessage(scope, file, action, action.Payload, s)
	if !ok {
		return nil, nil, nil, false
	}
	return newMessage(messageField("payload", 0, msg.fullName())), []string{"the " + msg.name + " the action takes"}, msg, true
}

// customResponse builds what the response of a custom action holds, with
// the comment of its field: the Result as the field result, or nothing when
// the action declares no Result.
//
// The Create action of Item on items/merge, declaring Result[*MergeRsp],
// gets
//
//	// MergeItemResponse is the response of ItemService.MergeItem.
//	message MergeItemResponse {
//	  // result is the MergeRsp the action answers with.
//	  MergeRsp result = 1;
//	}
func (g *generator) customResponse(scope *types.Scope, file *protoFile, action *dsl.Action, s jsonshape.Site) (*descriptorpb.DescriptorProto, []string, *message, bool) {
	if action.Result == "" || action.Result == dsl.PayloadEmpty {
		return newMessage(), nil, nil, true
	}
	msg, ok := g.typeMessage(scope, file, action, action.Result, s)
	if !ok {
		return nil, nil, nil, false
	}
	return newMessage(messageField("result", 0, msg.fullName())), []string{"the " + msg.name + " the action answers with"}, msg, true
}

// typeMessage resolves the message of the Go type a Payload or Result names,
// queued to be built and imported into file. An alias stands for the type it
// names, whose message it shares.
func (g *generator) typeMessage(scope *types.Scope, file *protoFile, action *dsl.Action, typeName string, s jsonshape.Site) (*message, bool) {
	obj, ok := scope.Lookup(strings.TrimPrefix(typeName, "*")).(*types.TypeName)
	if !ok {
		g.project.Report(s, "the %s action declares the type %s, which the model's package does not declare", action.Phase.Name(), typeName)
		return nil, false
	}
	if obj.IsAlias() {
		named, isNamed := types.Unalias(obj.Type()).(*types.Named)
		if !isNamed {
			g.project.Report(s, "the %s action declares %s, an alias of %s, which is not a named type; declare a struct type", action.Phase.Name(), typeName, types.Unalias(obj.Type()))
			return nil, false
		}
		obj = named.Obj()
	}
	msg := g.messageOf(obj)
	file.importOf(msg.file.registered)
	return msg, true
}

// standardMessages builds the request and response messages of a standard
// action, each with the comment of its fields: what they hold after the
// route parameters rpcMessages puts first, numbered in order after them.
// With X the model and x its field name (see modelFieldName):
//
//	Create:     ...Request { X x; }                                    ...Response { X x; }
//	Get:        ...Request { repeated string expand; uint32 depth; }   ...Response { X x; }
//	Update:     ...Request { X x; }                                    ...Response { X x; }
//	Patch:      ...Request { X x; FieldMask update_mask; }             ...Response { X x; }
//	Delete:     ...Request {}                                          ...Response {}
//	List:       ...Request { repeated Filter filters; repeated string sort_by; uint32 page; uint32 size;
//	                         string cursor_field; string cursor_value; bool cursor_next;
//	                         repeated string expand; uint32 depth; }    ...Response { repeated X items; int64 total; }
//	            with the nested Filter { string field = 1; string op = 2; repeated string values = 3; }
//	CreateMany, UpdateMany:
//	            ...Request { repeated X items; }                       ...Response { repeated X items; }
//	PatchMany:  ...Request { repeated Patch<X>Request items; }          ...Response { repeated X items; }
//	            with Patch<X>Request the request of the route's Patch, an item naming the record by its id
//	            (see patchRequest for a model declaring no Patch there)
//	DeleteMany: ...Request { repeated string ids; }                    ...Response {}
//
// So the Get action of a Record declaring Param("record"), its id put first
// by rpcMessages, gets
//
//	// GetRecordRequest is the request of RecordService.GetRecord.
//	message GetRecordRequest {
//	  // id is the id of the Record.
//	  string id = 1;
//
//	  // expand is the associations to expand, as the _expand query parameter names them.
//	  repeated string expand = 2;
//
//	  // depth is the depth of the expansion, as the _depth query parameter.
//	  uint32 depth = 3;
//	}
//
//	// GetRecordResponse is the response of RecordService.GetRecord.
//	message GetRecordResponse {
//	  // record is the Record found.
//	  Record record = 1;
//	}
func standardMessages(m *modelinfo.Model, model *message, file *protoFile, route string, action *dsl.Action) (request *descriptorpb.DescriptorProto, requestFields []string, response *descriptorpb.DescriptorProto, responseFields []string) {
	x := modelFieldName(m)
	switch action.Phase {
	case consts.Create:
		request = newMessage(modelField(x, model))
		requestFields = append(requestFields, "the "+m.ModelName+" to create")
		response = newMessage(modelField(x, model))
		responseFields = append(responseFields, "the "+m.ModelName+" created")
	case consts.Get:
		fields, comments, _ := queryFields(action.Phase)
		request = newMessage(fields...)
		requestFields = append(requestFields, comments...)
		response = newMessage(modelField(x, model))
		responseFields = append(responseFields, "the "+m.ModelName+" found")
	case consts.Update:
		request = newMessage(modelField(x, model))
		requestFields = append(requestFields, "the replacement")
		response = newMessage(modelField(x, model))
		responseFields = append(responseFields, "the "+m.ModelName+" as stored")
	case consts.Patch:
		file.importOf(fieldMaskProto)
		request = newMessage(modelField(x, model), messageField("update_mask", 0, wellKnownFieldMask))
		requestFields = append(requestFields, "the values to apply", "the fields of "+x+" to apply, named as the message names them")
		response = newMessage(modelField(x, model))
		responseFields = append(responseFields, "the "+m.ModelName+" as stored")
	case consts.Delete:
		request = newMessage()
		response = newMessage()
	case consts.List:
		fields, comments, nested := queryFields(action.Phase)
		request = newMessage(fields...)
		request.NestedType = nested
		requestFields = append(requestFields, comments...)
		response = newMessage(repeatedMessageField("items", model.fullName()), scalarField("total", 0, descriptorpb.FieldDescriptorProto_TYPE_INT64))
		responseFields = append(responseFields, "the "+m.ModelName+" records of the page", "the number of records the filters match, 0 under cursor pagination")
	case consts.CreateMany, consts.UpdateMany:
		request = newMessage(repeatedMessageField("items", model.fullName()))
		requestFields = append(requestFields, "the "+m.ModelName+" records to write")
		response = newMessage(repeatedMessageField("items", model.fullName()))
		responseFields = append(responseFields, "the "+m.ModelName+" records as stored")
	case consts.PatchMany:
		item := messageName(m, route, patchAction(m), "Request")
		request = newMessage(repeatedMessageField("items", "."+file.pkg+"."+item))
		requestFields = append(requestFields, "the patches, each a "+item+" naming the "+m.ModelName+" it applies to by its id")
		response = newMessage(repeatedMessageField("items", model.fullName()))
		responseFields = append(responseFields, "the "+m.ModelName+" records as stored")
	case consts.DeleteMany:
		request = newMessage(repeatedStringField("ids", 0))
		requestFields = append(requestFields, "the ids of the "+m.ModelName+" records to delete")
		response = newMessage()
	}
	return request, requestFields, response, responseFields
}

// queryFields returns the fields a request of the phase holds for the query
// parameters gst reads on it, with their comments and nested types, numbered
// by the caller: List's filters, orderings, pagination, cursor and
// expansion, with the nested Filter of the filters; Get's expansion; nothing
// for the other phases.
//
// The List action of Record on records gets, its fields numbered from 1 as
// the route has no parameter,
//
//	// ListRecordRequest is the request of RecordService.ListRecord.
//	message ListRecordRequest {
//	  // filters is the filters to apply, each one field[op]=value of the HTTP query.
//	  repeated Filter filters = 1;
//
//	  // sort_by is the orderings, as the _sort_by query parameter names them.
//	  repeated string sort_by = 2;
//
//	  // page is the page to list, as the _page query parameter.
//	  uint32 page = 3;
//
//	  // size is the page size, as the _size query parameter, read by a model embedding model.Pagination or model.Cursor alone: 20 when unset and at most 100; any other model ignores it and lists at most 1000 records.
//	  uint32 size = 4;
//
//	  // cursor_field is the cursor column, as the _cursor_field query parameter.
//	  string cursor_field = 5;
//
//	  // cursor_value is the cursor position, as the _cursor_value query parameter.
//	  string cursor_value = 6;
//
//	  // cursor_next is whether to list past the cursor, as the _cursor_next query parameter.
//	  bool cursor_next = 7;
//
//	  // expand is the associations to expand, as the _expand query parameter names them.
//	  repeated string expand = 8;
//
//	  // depth is the depth of the expansion, as the _depth query parameter.
//	  uint32 depth = 9;
//
//	  // Filter is one filter of filters: field names the column by its query name, op is the operator, none for the equality every model answers, and values is its value, several for in and notin.
//	  message Filter {
//	    string field = 1;
//
//	    string op = 2;
//
//	    repeated string values = 3;
//	  }
//	}
func queryFields(phase consts.Phase) (fields []*descriptorpb.FieldDescriptorProto, comments []string, nested []*descriptorpb.DescriptorProto) {
	expansion := []*descriptorpb.FieldDescriptorProto{
		repeatedStringField("expand", 0),
		scalarField("depth", 0, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
	}
	expansionComments := []string{
		"the associations to expand, as the _expand query parameter names them",
		"the depth of the expansion, as the _depth query parameter",
	}
	switch phase {
	case consts.Get:
		return expansion, expansionComments, nil
	case consts.List:
		filter := newMessage(stringField("field", 1), stringField("op", 2), repeatedStringField("values", 3))
		filter.Name = new(filterMessage)
		fields = append([]*descriptorpb.FieldDescriptorProto{
			repeatedMessageField("filters", filterMessage),
			repeatedStringField("sort_by", 0),
			scalarField("page", 0, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			scalarField("size", 0, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			stringField("cursor_field", 0),
			stringField("cursor_value", 0),
			scalarField("cursor_next", 0, descriptorpb.FieldDescriptorProto_TYPE_BOOL),
		}, expansion...)
		comments = append([]string{
			"the filters to apply, each one field[op]=value of the HTTP query",
			"the orderings, as the _sort_by query parameter names them",
			"the page to list, as the _page query parameter",
			"the page size, as the _size query parameter, read by a model embedding model.Pagination or model.Cursor alone: " + strconv.Itoa(urlquery.DefaultPageSize) + " when unset and at most " + strconv.Itoa(urlquery.MaxPageSize) + "; any other model ignores it and lists at most " + strconv.Itoa(urlquery.UnpagedLimit) + " records",
			"the cursor column, as the _cursor_field query parameter",
			"the cursor position, as the _cursor_value query parameter",
			"whether to list past the cursor, as the _cursor_next query parameter",
		}, expansionComments...)
		return fields, comments, []*descriptorpb.DescriptorProto{filter}
	}
	return nil, nil, nil
}

// numberFields numbers the fields of the request or response message desc,
// to be named name in file, the way a model's untagged fields are numbered
// (see nextNumbers): a field the committed definition under pb/ holds keeps
// its number, whatever position it holds now, and any other takes the next
// free number; the fields, and comments with them, are then ordered by
// number. The nested messages keep the numbers they declare. GetNoteRequest,
// holding id, expand and depth in that order, is numbered 1, 2, 3 by a first
// generation; when the committed file holds expand = 1 and id = 2 it is
// numbered expand = 1, id = 2, depth = 3 and printed in that order, and when
// the committed file holds id = 1, expand = 2 and reserves 3, depth takes 4.
// It returns the comments in the order of the fields, or false once no
// number is left, the message reported.
func (g *generator) numberFields(s jsonshape.Site, file *protoFile, name string, desc *descriptorpb.DescriptorProto, comments []string) ([]string, bool) {
	numbers := make(map[int32]string, len(desc.Field))
	next := g.nextNumbers(file, name, numbers, false)
	for _, field := range desc.Field {
		number, ok := next(field.GetName())
		if !ok {
			g.project.Report(s, "the field %s of message %s has no field number left: every one is one the committed %s/%s reserves; lift a reservation, or delete the file to start over", field.GetName(), name, ggconst.DirPB, file.name)
			return nil, false
		}
		numbers[number] = field.GetName()
		field.Number = new(number)
	}
	order := make([]int, len(desc.Field))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(desc.Field[a].GetNumber(), desc.Field[b].GetNumber()) })
	fields := make([]*descriptorpb.FieldDescriptorProto, len(desc.Field))
	ordered := make([]string, len(comments))
	for i, from := range order {
		fields[i] = desc.Field[from]
		ordered[i] = comments[from]
	}
	desc.Field = fields
	return ordered, true
}

// filterMessage names the nested Filter message of a List request (see
// queryFields) and filterComment is its leading comment; the Filter is the
// one nested message a request or response message holds.
const (
	filterMessage = "Filter"
	filterComment = "Filter is one filter of filters: field names the column by its query name, op is the operator, none for the equality every model answers, and values is its value, several for in and notin."
)

// addRPCMessage appends a request or response message to file, with the
// comment of the message, one per field, each field's comment starting with
// its name, and the comment of its nested Filter, when it holds one.
func (g *generator) addRPCMessage(file *protoFile, desc *descriptorpb.DescriptorProto, comment string, fieldComments []string) {
	index := int32Index(len(file.messages))
	for i, text := range fieldComments {
		file.comment([]int32{fileMessagesTag, index, messageFieldsTag, int32Index(i)}, desc.Field[i].GetName()+" is "+text+".")
	}
	for i, nested := range desc.NestedType {
		var text string
		if nested.GetName() == filterMessage {
			text = filterComment
		}
		file.comment([]int32{fileMessagesTag, index, messageNestedTypesTag, int32Index(i)}, text)
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
// names, to be numbered by numberInOrder.
func repeatedMessageField(name string, typeName string) *descriptorpb.FieldDescriptorProto {
	field := messageField(name, 0, typeName)
	field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return field
}

// modelField returns the singular field carrying the model's message, to be
// numbered by numberInOrder.
func modelField(name string, model *message) *descriptorpb.FieldDescriptorProto {
	return messageField(name, 0, model.fullName())
}
