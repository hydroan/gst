package openapigen

import (
	"net/http"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
)

// sseMediaType is the media type of a Server-Sent Events response.
const sseMediaType = "text/event-stream"

// setSSE documents the SSE action (GET /{path}): a long-lived text/event-stream
// response the client consumes through an EventSource. The action delegates to
// a custom service that reads its own query parameters, so only the path
// parameters are documented; the response is the raw event stream, not the
// JSON envelope.
func setSSE[M gst.Model, REQ gst.Request, RSP gst.Response](path string, pathItem *openapi3.PathItem) {
	custom := !model.AreTypesEqual[M, REQ, RSP]()

	op := newOperation(path, apiaction.SSE, reflect.TypeFor[M](), custom)
	op.Responses = sseStreamResponses(custom)
	pathItem.Get = op
}

// sseStreamResponses documents the event stream: a 200 response whose body is
// a text/event-stream of SSE frames, and the failures the stream answers
// before it opens (see responsesOf), custom when the action's types are the
// project's own.
func sseStreamResponses(custom bool) *openapi3.Responses {
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
	return responsesOf(http.StatusOK, &openapi3.ResponseRef{Value: response}, apiaction.SSE, custom)
}
