package sse_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/sse"
	"github.com/stretchr/testify/require"
)

// TestServe_StreamsOnTheResponseTheContextCarries pins the entry a service
// opens its stream through: the events it sends travel on the response of the
// request its context serves.
func TestServe_StreamsOnTheResponseTheContextCarries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/events", func(c *gin.Context) {
		ctx := gst.NewServiceContext(c, c.Request.Context(), "")
		if err := sse.Serve(ctx, func(conn *sse.Conn) error {
			return conn.Send(sse.Event{Event: "greeting", Data: "hello"})
		}); err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/events", nil)
	require.NoError(t, err)
	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()
	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)

	require.Contains(t, rsp.Header.Get("Content-Type"), "text/event-stream")
	require.Contains(t, string(body), "event: greeting\n")
	require.Contains(t, string(body), "hello")
}

// TestServe_RefusesAContextCarryingNoHTTPResponse pins what a stream opened
// where there is nothing to stream on comes to: the callback never runs, the
// error says why, and the context records the attempt for the transport to
// refuse the request by, a nil context having nothing to record on.
func TestServe_RefusesAContextCarryingNoHTTPResponse(t *testing.T) {
	for name, ctx := range map[string]*gst.ServiceContext{
		"a context served over no HTTP exchange": gst.NewServiceContext(nil, nil, ""),
		"a nil context":                          nil,
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			err := sse.Serve(ctx, func(*sse.Conn) error {
				called = true
				return nil
			})

			require.EqualError(t, err, "service context carries no HTTP response to stream on")
			require.False(t, called)
			require.Equal(t, ctx != nil, gst.HTTPOnlyMethodCalled(ctx))
		})
	}
}
