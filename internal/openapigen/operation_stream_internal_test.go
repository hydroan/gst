package openapigen

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// streamedNote is the message the clients of the Stream actions documented
// in this file send, streamedEvent the one their servers send; the actions
// are declared on openapiActionModel.
type streamedNote struct {
	// Note is what the client says.
	Note string `json:"note"`
}

type streamedEvent struct {
	Seq int64 `json:"seq"`
}

// The services below declare the three kinds of stream, each on the messages
// its route is registered with: the Stream method of a route's service is
// what tells the document which side sends one message and which a stream.
type (
	noteWatcher struct {
		service.Base[*openapiActionModel, *streamedNote, *streamedEvent]
	}
	eventTailer struct {
		service.Base[*openapiActionModel, *model.Empty, *streamedEvent]
	}
	noteUploader struct {
		service.Base[*openapiActionModel, *streamedNote, *streamedEvent]
	}
	noteIngester struct {
		service.Base[*openapiActionModel, *streamedNote, *model.Empty]
	}
	noteChatter struct {
		service.Base[*openapiActionModel, *streamedNote, *streamedEvent]
	}
)

func (*noteWatcher) Stream(*types.ServiceContext, *streamedNote, *types.ServerStream[*streamedEvent]) error {
	return nil
}

func (*eventTailer) Stream(*types.ServiceContext, *model.Empty, *types.ServerStream[*streamedEvent]) error {
	return nil
}

func (*noteUploader) Stream(*types.ServiceContext, *types.ClientStream[*streamedNote]) (*streamedEvent, error) {
	return &streamedEvent{}, nil
}

func (*noteIngester) Stream(*types.ServiceContext, *types.ClientStream[*streamedNote]) (*model.Empty, error) {
	return &model.Empty{}, nil
}

func (*noteChatter) Stream(*types.ServiceContext, *types.BidiStream[*streamedNote, *streamedEvent]) error {
	return nil
}

// The routes the stream services are registered on, the way the router
// registers them. streamUnservedRoute has no service.
const (
	streamWatchRoute    = "/api/stream-docs/:feed/watch"
	streamTailRoute     = "/api/stream-docs/tail"
	streamUploadRoute   = "/api/stream-docs/upload"
	streamIngestRoute   = "/api/stream-docs/ingest"
	streamChatRoute     = "/api/stream-docs/chat"
	streamUnservedRoute = "/api/stream-docs/unserved"
)

// The services register once, in init: the registry refuses the second
// registration of a route, which a test run again would make.
func init() {
	service.RegisterInstance[*openapiActionModel, *streamedNote, *streamedEvent](apiaction.Stream, streamWatchRoute, &noteWatcher{})
	service.RegisterInstance[*openapiActionModel, *model.Empty, *streamedEvent](apiaction.Stream, streamTailRoute, &eventTailer{})
	service.RegisterInstance[*openapiActionModel, *streamedNote, *streamedEvent](apiaction.Stream, streamUploadRoute, &noteUploader{})
	service.RegisterInstance[*openapiActionModel, *streamedNote, *model.Empty](apiaction.Stream, streamIngestRoute, &noteIngester{})
	service.RegisterInstance[*openapiActionModel, *streamedNote, *streamedEvent](apiaction.Stream, streamChatRoute, &noteChatter{})
}

// TestSetDocumentsAStreamAsTheConnectionItsRequestOpens pins the entry of a
// Stream action: a GET answered 101, with the failures of a handshake
// refused beside it; a description gaining the paragraph that says how the
// stream travels, what each side sends, by the kind of stream the action's
// service declares, and how it ends; and the messages documented as schema
// components, the server's the content of the 101.
func TestSetDocumentsAStreamAsTheConnectionItsRequestOpens(t *testing.T) {
	note, event := uniqueComponentName(reflect.TypeFor[*streamedNote]()), uniqueComponentName(reflect.TypeFor[*streamedEvent]())
	tests := []struct {
		name           string
		path           string
		set            func()
		client, server string
		// answers reports whether the server sends a message, which the 101
		// then documents.
		answers bool
	}{
		{
			name: "a server stream",
			path: "/api/stream-docs/{feed}/watch",
			set: func() {
				set[*openapiActionModel, *streamedNote, *streamedEvent](streamWatchRoute, true, apiaction.Stream)
			},
			client:  "The client sends one " + note + " message, the request of the stream.",
			server:  "The server sends a stream of " + event + " messages.",
			answers: true,
		},
		{
			name: "a server stream taking no request",
			path: streamTailRoute,
			set: func() {
				set[*openapiActionModel, *model.Empty, *streamedEvent](streamTailRoute, true, apiaction.Stream)
			},
			client:  "The client sends no message.",
			server:  "The server sends a stream of " + event + " messages.",
			answers: true,
		},
		{
			name: "a client stream",
			path: streamUploadRoute,
			set: func() {
				set[*openapiActionModel, *streamedNote, *streamedEvent](streamUploadRoute, true, apiaction.Stream)
			},
			client:  "The client sends a stream of " + note + " messages and ends it with a frame holding null.",
			server:  "The server answers with one " + event + " message once the client has ended its stream.",
			answers: true,
		},
		{
			name: "a client stream answering no response",
			path: streamIngestRoute,
			set: func() {
				set[*openapiActionModel, *streamedNote, *model.Empty](streamIngestRoute, true, apiaction.Stream)
			},
			client: "The client sends a stream of " + note + " messages and ends it with a frame holding null.",
			server: "The server sends no message.",
		},
		{
			name: "a bidirectional stream",
			path: streamChatRoute,
			set: func() {
				set[*openapiActionModel, *streamedNote, *streamedEvent](streamChatRoute, true, apiaction.Stream)
			},
			client:  "The client sends a stream of " + note + " messages and ends it with a frame holding null.",
			server:  "The server sends a stream of " + event + " messages.",
			answers: true,
		},
		{
			name: "an action whose service declares no stream",
			path: streamUnservedRoute,
			set: func() {
				set[*openapiActionModel, *streamedNote, *streamedEvent](streamUnservedRoute, true, apiaction.Stream)
			},
			client:  "The client sends " + note + " messages.",
			server:  "The server sends " + event + " messages.",
			answers: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.set()

			item := doc.Paths.Value(tt.path)
			require.NotNil(t, item, "%s is missing from the document", tt.path)
			op := item.Get
			require.NotNil(t, op, "GET %s is missing from the document", tt.path)
			require.Equal(t, []string{"101", "400", "default"}, slices.Sorted(maps.Keys(op.Responses.Map())))
			exchange := strings.Join([]string{streamOpening, tt.client, tt.server, streamClosing}, " ")
			require.True(t, strings.HasSuffix(op.Description, "\n\n"+exchange), "the description is %q, want it to end with %q", op.Description, exchange)

			opened := op.Responses.Status(http.StatusSwitchingProtocols).Value
			if !tt.answers {
				require.Empty(t, opened.Content)
				return
			}
			require.Equal(t, "#/components/schemas/"+event, opened.Content.Get("application/json").Schema.Ref)
		})
	}

	watch := doc.Paths.Value("/api/stream-docs/{feed}/watch").Get
	require.Len(t, watch.Parameters, 1)
	require.Equal(t, "feed", watch.Parameters[0].Value.Name, "the handshake carries the route parameter")
	require.Equal(t, []string{"note"}, propertyNames(doc.Components.Schemas[note].Value), "a message is documented bare of the envelope")
	require.Equal(t, []string{"seq"}, propertyNames(doc.Components.Schemas[event].Value))
}

// TestStreamExchangeSaysWhatEachSideSends guards the examples of
// streamExchange: the sentences of a server stream and of a client stream
// taking a SampleReq and answering SampleEvent messages.
func TestStreamExchangeSaysWhatEachSideSends(t *testing.T) {
	server := streamExchange[*openapiActionModel, *streamedNote, *streamedEvent](streamWatchRoute, "SampleReq", "SampleEvent")
	require.Contains(t, server, " The client sends one SampleReq message, the request of the stream. The server sends a stream of SampleEvent messages. ")

	client := streamExchange[*openapiActionModel, *streamedNote, *streamedEvent](streamUploadRoute, "SampleReq", "SampleEvent")
	require.Contains(t, client, " The client sends a stream of SampleReq messages and ends it with a frame holding null. The server answers with one SampleEvent message once the client has ended its stream. ")

	unserved := streamExchange[*openapiActionModel, *streamedNote, *streamedEvent](streamUnservedRoute, "SampleReq", "")
	require.Contains(t, unserved, " The client sends SampleReq messages. The server sends messages. ", "a message without a name is one all the same")
}
