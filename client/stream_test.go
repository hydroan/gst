package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestStreamDeliversEventsInOrder(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: tick\ndata: one\n\n")
		fmt.Fprint(w, "data: two\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	var events []types.Event
	err = cli.Stream(http.MethodPost, "/api/records/stream", nil, func(event types.Event) error {
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

func TestStreamSurfacesEnvelopeRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":403,"msg":"permission denied"}`)
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	err = cli.Stream(http.MethodPost, "/api/records/stream", nil, func(types.Event) error { return nil })
	var respErr *client.Error
	require.ErrorAs(t, err, &respErr)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
}

func TestStreamRequiresCallback(t *testing.T) {
	cli, err := client.New("http://127.0.0.1:1")
	require.NoError(t, err)
	require.Error(t, cli.Stream(http.MethodPost, "/api/records/stream", nil, nil))
}
