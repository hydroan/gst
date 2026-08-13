package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// sseMediaType is the media type of a Server-Sent Events response.
const sseMediaType = "text/event-stream"

// setSSE documents the SSE action (GET /{path}): a long-lived text/event-stream
// response the client consumes through an EventSource. The action delegates to
// a custom service that reads its own query parameters, so only the path
// parameters are documented; the response is the raw event stream, not the
// JSON envelope.
func setSSE[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.SSE),
		Summary:     summary(path, consts.SSE, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.SSE, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.SSE, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   sseStreamResponses(),
	}
}

// sseStreamResponses documents the event stream: a 200 response whose body is
// a text/event-stream of SSE frames.
func sseStreamResponses() *openapi3.Responses {
	streamSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:        &openapi3.Types{openapi3.TypeString},
			Description: "A stream of Server-Sent Events frames.",
		},
	}
	response := openapi3.NewResponse().
		WithDescription("The Server-Sent Events stream.").
		WithContent(openapi3.Content{
			sseMediaType: openapi3.NewMediaType().WithSchemaRef(streamSchema),
		})
	return openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: response}))
}
