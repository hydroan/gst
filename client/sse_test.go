package client_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/sse"
	"github.com/stretchr/testify/require"
)

// TestSSEDeliversEventsInOrder proves SSE asks for an event stream and Recv
// returns its events in the order the server sent them, io.EOF after the
// last.
func TestSSEDeliversEventsInOrder(t *testing.T) {
	var gotAccept string
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: tick\ndata: one\n\n")
		fmt.Fprint(w, "data: two\n\n")
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodPost, "/api/records/stream", nil)
	require.NoError(t, err)
	defer events.Close()
	got := drain(t, events)
	require.Equal(t, "text/event-stream", gotAccept)
	require.Len(t, got, 2)
	require.Equal(t, "tick", got[0].Event)
	require.Equal(t, "one", got[0].Data)
	require.Equal(t, "two", got[1].Data)
}

// TestSSEParsesPerSpec pins how Recv reads a stream, the WHATWG processing
// model case by case.
func TestSSEParsesPerSpec(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []sse.Event
	}{
		{
			name: "comment lines and heartbeats are invisible",
			body: ": ping\n\ndata: one\n\n: ping\n\n",
			want: []sse.Event{{Data: "one"}},
		},
		{
			name: "event id sticks across events until replaced",
			body: "id: 1\ndata: one\n\ndata: two\n\nid: 2\ndata: three\n\n",
			want: []sse.Event{{ID: "1", Data: "one"}, {ID: "1", Data: "two"}, {ID: "2", Data: "three"}},
		},
		{
			name: "id containing NUL is ignored",
			body: "id: 1\ndata: one\n\nid: bad\x00id\ndata: two\n\n",
			want: []sse.Event{{ID: "1", Data: "one"}, {ID: "1", Data: "two"}},
		},
		{
			name: "only one leading space is stripped from the value",
			body: "data:  padded\n\n",
			want: []sse.Event{{Data: " padded"}},
		},
		{
			name: "field without a colon has an empty value",
			body: "data\n\n",
			want: []sse.Event{{Data: ""}},
		},
		{
			name: "event without data is not dispatched and drops its type",
			body: "event: skipped\n\ndata: one\n\n",
			want: []sse.Event{{Data: "one"}},
		},
		{
			name: "retry only accepts all-digit values",
			body: "retry: 3000\ndata: one\n\nretry: -1\nretry: 12ab\ndata: two\n\n",
			want: []sse.Event{{Retry: 3000, Data: "one"}, {Retry: 3000, Data: "two"}},
		},
		{
			name: "multi-line data is joined with newlines",
			body: "data: one\ndata: two\n\n",
			want: []sse.Event{{Data: "one\ntwo"}},
		},
		{
			name: "cr and crlf line terminators are accepted",
			body: "data: one\r\n\r\ndata: two\r\rdata: three\n\n",
			want: []sse.Event{{Data: "one"}, {Data: "two"}, {Data: "three"}},
		},
		{
			name: "a leading byte order mark is stripped",
			body: "\uFEFFdata: one\n\n",
			want: []sse.Event{{Data: "one"}},
		},
		{
			name: "an event pending at end of stream is discarded",
			body: "data: one\n\ndata: dangling",
			want: []sse.Event{{Data: "one"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tt.body)
			}))
			srv.Start()

			cli, err := client.New(srv.URL)
			require.NoError(t, err)

			events, err := cli.SSE(t.Context(), http.MethodGet, "/api/records/stream", nil)
			require.NoError(t, err)
			defer events.Close()
			require.Equal(t, tt.want, drain(t, events))
		})
	}
}

// TestSSESurfacesEnvelopeRejection proves a JSON answer refusing the request
// surfaces as the *Error of its envelope, with no stream returned.
func TestSSESurfacesEnvelopeRejection(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"msg":"permission denied"}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodPost, "/api/records/stream", nil)
	require.Nil(t, events)
	var respErr *client.Error
	require.ErrorAs(t, err, &respErr)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
	require.Equal(t, "permission denied", respErr.Msg)
}

// TestSSEAnswersNoEventForAJSONSuccess proves a JSON answer accepting the
// request returns a stream carrying no event.
func TestSSEAnswersNoEventForAJSONSuccess(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"msg":"success"}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodGet, "/api/records/stream", nil)
	require.NoError(t, err)
	defer events.Close()
	_, err = events.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// TestSSEEndsWhenContextEnds proves the context the stream was opened on
// bounds it: once it ends, Recv fails with its error.
func TestSSEEndsWhenContextEnds(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(endlessEvents))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events, err := cli.SSE(ctx, http.MethodGet, "/api/records/stream", nil)
	require.NoError(t, err)
	defer events.Close()
	_, err = events.Recv()
	require.NoError(t, err)
	cancel()
	_, err = events.Recv()
	require.ErrorIs(t, err, context.Canceled)
}

// TestEventStreamCloseEndsTheStream proves Close ends a stream the server
// would keep open: the server sees its client leave.
func TestEventStreamCloseEndsTheStream(t *testing.T) {
	left := make(chan struct{})
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endlessEvents(w, r)
		close(left)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	events, err := cli.SSE(t.Context(), http.MethodGet, "/api/records/stream", nil)
	require.NoError(t, err)
	_, err = events.Recv()
	require.NoError(t, err)
	require.NoError(t, events.Close())
	select {
	case <-left:
	case <-time.After(5 * time.Second):
		t.Fatal("the server must see the client leave once the stream is closed")
	}
}

// endlessEvents sends one event and then keeps the stream open with
// heartbeats until the client goes away, so that only the client ends it.
func endlessEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "data: one\n\n")
	for {
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
		fmt.Fprint(w, ": ping\n\n")
	}
}

// drain returns the events of the stream up to its end.
func drain(t *testing.T, events *client.EventStream) []sse.Event {
	t.Helper()

	var got []sse.Event
	for {
		event, err := events.Recv()
		if errors.Is(err, io.EOF) {
			return got
		}
		require.NoError(t, err)
		got = append(got, event)
	}
}
