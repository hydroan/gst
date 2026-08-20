package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/sse"
	"github.com/stretchr/testify/require"
)

func TestStreamDeliversEventsInOrder(t *testing.T) {
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

	var events []sse.Event
	err = cli.Stream(http.MethodPost, "/api/records/stream", nil, func(event sse.Event) error {
		events = append(events, event)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", gotAccept)
	require.Len(t, events, 2)
	require.Equal(t, "tick", events[0].Event)
	require.Equal(t, "one", events[0].Data)
	require.Equal(t, "two", events[1].Data)
}

func TestStreamParsesPerSpec(t *testing.T) {
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

			var events []sse.Event
			err = cli.Stream(http.MethodGet, "/api/records/stream", nil, func(event sse.Event) error {
				events = append(events, event)
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, events)
		})
	}
}

func TestStreamStopsOnCallbackError(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: one\n\ndata: two\n\n")
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	var seen int
	err = cli.Stream(http.MethodGet, "/api/records/stream", nil, func(sse.Event) error {
		seen++
		return errors.New("stop after the first event")
	})
	require.EqualError(t, err, "stop after the first event")
	require.Equal(t, 1, seen)
}

func TestStreamSurfacesEnvelopeRejection(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":403,"msg":"permission denied"}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	err = cli.Stream(http.MethodPost, "/api/records/stream", nil, func(sse.Event) error { return nil })
	var respErr *client.Error
	require.ErrorAs(t, err, &respErr)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
}

func TestStreamRequiresCallback(t *testing.T) {
	cli, err := client.New("http://127.0.0.1:1")
	require.NoError(t, err)
	require.Error(t, cli.Stream(http.MethodPost, "/api/records/stream", nil, nil))
}
