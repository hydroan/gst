package openapigen

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
	"github.com/hydroan/gst/internal/types"
)

// The sentences opening and closing the paragraph a Stream action's
// description gains (see streamExchange): how the stream travels, and how it
// ends.
const (
	streamOpening = "The request opens a WebSocket connection that carries the stream, every message one text frame holding a JSON document."
	streamClosing = "The server closes the connection once the stream is over: with the status 1000, or after a failure with the status 4000 plus the HTTP status the failure answers with, its message as the reason."
)

// setStream documents the Stream action (GET /{path}): the request opens a
// WebSocket connection, answered 101, that carries the messages of the
// stream. route is the path the way the router registers it, which the
// action's service is registered under.
//
// The description says what each side sends (see streamExchange). The
// messages are documented as they travel, bare of the envelope the response
// of another action is wrapped in: the server's as the content of the 101,
// the client's by name in the description, an OpenAPI document holding no
// place for what the request of a GET goes on to send.
func setStream[M types.Model, REQ types.Request, RSP types.Response](route, path string, pathItem *openapi3.PathItem) {
	custom := !model.AreTypesEqual[M, REQ, RSP]()
	sent, _ := streamMessage[REQ]()
	answered, answeredSchema := streamMessage[RSP]()

	op := newOperation(path, apiaction.Stream, reflect.TypeFor[M](), custom)
	op.Description += "\n\n" + streamExchange[M, REQ, RSP](route, sent, answered)
	opened := openapi3.NewResponse().WithDescription("The WebSocket connection of the stream is open.")
	if answeredSchema != nil {
		opened = opened.WithContent(openapi3.NewContentWithJSONSchemaRef(answeredSchema))
	}
	op.Responses = responsesOf(http.StatusSwitchingProtocols, &openapi3.ResponseRef{Value: opened}, apiaction.Stream, custom)
	pathItem.Get = op
}

// streamMessage documents T, the type of the messages one side of a Stream
// action sends, and returns the name the description refers to the type by
// and its schema: the schema component it registers for a named type and a
// reference to it, no name and the schema itself for a type without one. A
// side sending no message, *model.Empty, has neither.
func streamMessage[T any]() (name string, schema *openapi3.SchemaRef) {
	if model.IsEmpty[T]() {
		return "", nil
	}
	schema = newSchemaRefWithDocs(*new(T))
	if name = uniqueComponentName(reflect.TypeFor[T]()); name == "" || schema == nil {
		return "", schema
	}

	docMutex.Lock()
	defer docMutex.Unlock()
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = openapi3.Schemas{}
	}
	if _, ok := doc.Components.Schemas[name]; !ok {
		doc.Components.Schemas[name] = schema
	}
	return name, &openapi3.SchemaRef{Ref: "#/components/schemas/" + name}
}

// streamExchange returns the paragraph the description of the Stream action
// registered on route gains: how the stream travels (streamOpening), what
// the client and the server send, and how it ends (streamClosing). Which
// side sends one message and which a stream of them is the kind of stream
// the Stream method of the action's service declares (see
// types.ServerStreamer and its kind); sent and answered name the messages of
// the two sides, "" one without a name. For a server stream taking a
// SampleReq and answering SampleEvent messages the two sentences read
//
//	The client sends one SampleReq message, the request of the stream.
//	The server sends a stream of SampleEvent messages.
//
// for a client stream
//
//	The client sends a stream of SampleReq messages and ends it with a frame holding null.
//	The server answers with one SampleEvent message once the client has ended its stream.
//
// and for a bidirectional stream the first of the one and the second of the
// other. A side declaring no message, *model.Empty, sends none, "The client
// sends no message.", and an action whose service declares no Stream method
// is described by its messages alone, "The client sends SampleReq messages."
func streamExchange[M types.Model, REQ types.Request, RSP types.Response](route, sent, answered string) string {
	svc := service.Resolve[M, REQ, RSP](service.Key(apiaction.Stream, route))
	_, serverStream := svc.(types.ServerStreamer[REQ, RSP])
	_, clientStream := svc.(types.ClientStreamer[REQ, RSP])
	_, bidiStream := svc.(types.BidiStreamer[REQ, RSP])
	// messages names the messages of a side, in the plural, and message one
	// of them.
	message := func(name string) string { return strings.TrimSpace(name + " message") }
	messages := func(name string) string { return message(name) + "s" }

	var client, server string
	switch {
	case model.IsEmpty[REQ]():
		client = "The client sends no message."
	case clientStream || bidiStream:
		client = "The client sends a stream of " + messages(sent) + " and ends it with a frame holding null."
	case serverStream:
		client = "The client sends one " + message(sent) + ", the request of the stream."
	default:
		client = "The client sends " + messages(sent) + "."
	}
	switch {
	case model.IsEmpty[RSP]():
		server = "The server sends no message."
	case serverStream || bidiStream:
		server = "The server sends a stream of " + messages(answered) + "."
	case clientStream:
		server = "The server answers with one " + message(answered) + " once the client has ended its stream."
	default:
		server = "The server sends " + messages(answered) + "."
	}
	return strings.Join([]string{streamOpening, client, server, streamClosing}, " ")
}
