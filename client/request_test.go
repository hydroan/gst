package client_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// newEnvelopeServer returns a test server answering with a gst envelope and a
// capture of the last request for assertions.
func newEnvelopeServer(t *testing.T, status int, envelope string) (*httptest.Server, *http.Request) {
	t.Helper()

	captured := new(http.Request)
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = *r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, envelope)
	}))
	srv.Start()
	return srv, captured
}

func TestDoParsesSuccessEnvelope(t *testing.T) {
	srv, captured := newEnvelopeServer(t, http.StatusOK,
		`{"code":0,"msg":"success","data":{"name":"sample"},"trace_id":"trace-1"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	resp, err := cli.Do(t.Context(), http.MethodPost, "/api/records", map[string]string{"name": "sample"},
		client.WithQuery("kind", "sample"), client.WithPage(1, 10))
	require.NoError(t, err)
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "trace-1", resp.TraceID)
	require.JSONEq(t, `{"name":"sample"}`, string(resp.Data))

	require.Equal(t, "/api/records", captured.URL.Path)
	require.Equal(t, "sample", captured.URL.Query().Get("kind"))
	require.Equal(t, "1", captured.URL.Query().Get("_page"))
	require.Equal(t, "10", captured.URL.Query().Get("_size"))
}

func TestDoReturnsStructuredErrorOnRejection(t *testing.T) {
	srv, _ := newEnvelopeServer(t, http.StatusForbidden,
		`{"code":403,"msg":"permission denied","trace_id":"trace-2"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	_, err = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr), "error: %v", err)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
	require.Equal(t, 403, respErr.Code)
	require.Equal(t, "permission denied", respErr.Msg)
	require.Equal(t, "trace-2", respErr.TraceID)
	require.NotEmpty(t, respErr.Body)
}

func TestDoReturnsStructuredErrorOnBusinessCodeWith2xx(t *testing.T) {
	srv, _ := newEnvelopeServer(t, http.StatusOK, `{"code":1001,"msg":"sample failure"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	_, err = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr))
	require.Equal(t, http.StatusOK, respErr.StatusCode)
	require.Equal(t, 1001, respErr.Code)
}

func TestClientKeepsSessionCookieAcrossRequests(t *testing.T) {
	// The login-shaped first request sets a cookie; the second request must
	// carry it back automatically through the client's cookie jar.
	var gotCookie string
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/enter" {
			http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sample-session", Path: "/"})
		}
		if cookie, err := r.Cookie("session_id"); err == nil {
			gotCookie = cookie.Value
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success"}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	resp, err := cli.Do(t.Context(), http.MethodPost, "/api/enter", nil)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Cookies)

	_, err = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
	require.NoError(t, err)
	require.Equal(t, "sample-session", gotCookie)
}

func TestWithHeaderMergesIntoDefaultHeaders(t *testing.T) {
	srv, captured := newEnvelopeServer(t, http.StatusOK, `{"code":0}`)

	header := http.Header{}
	header.Set("X-Forwarded-Proto", "https")
	cli, err := client.New(srv.URL, client.WithHeader(header))
	require.NoError(t, err)

	_, err = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
	require.NoError(t, err)
	require.Equal(t, "https", captured.Header.Get("X-Forwarded-Proto"))
	// Defaults set by New must survive a WithHeader merge.
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
}

func TestDoEndsWhenContextEnds(t *testing.T) {
	// The handler holds its answer until the client goes away, so the only
	// thing that can end the call is its context.
	var disconnected atomic.Bool
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		disconnected.Store(true)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = cli.Do(ctx, http.MethodGet, "/api/records", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// The server sees the call end as well: the connection is cut with it.
	require.Eventually(t, disconnected.Load, time.Second, 10*time.Millisecond)
}

func TestDoRefusesNilContext(t *testing.T) {
	cli, err := client.New("http://127.0.0.1:1")
	require.NoError(t, err)

	var ctx context.Context
	_, err = cli.Do(ctx, http.MethodGet, "/api/records", nil)
	require.ErrorContains(t, err, "nil Context")
}

func TestRequestCarriesTraceOfContext(t *testing.T) {
	srv, captured := newEnvelopeServer(t, http.StatusOK, `{"code":0}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	t.Run("the trace id stamped on the context", func(t *testing.T) {
		ctx := execctx.WithTraceID(t.Context(), "trace-sample")
		_, err := cli.Do(ctx, http.MethodGet, "/api/records", nil)
		require.NoError(t, err)
		require.Equal(t, "trace-sample", captured.Header.Get(consts.HEADER_TRACE_ID))
		require.Empty(t, captured.Header.Get("traceparent"))
	})

	t.Run("the span open on the context", func(t *testing.T) {
		// The propagator is the one the framework installs when tracing is
		// enabled; the test process runs without tracing, so it is installed
		// here for the span this test opens.
		restore := otel.GetTextMapPropagator()
		otel.SetTextMapPropagator(propagation.TraceContext{})
		t.Cleanup(func() { otel.SetTextMapPropagator(restore) })
		provider := sdktrace.NewTracerProvider()
		t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
		ctx, span := provider.Tracer("client-test").Start(t.Context(), "caller")
		defer span.End()

		_, err := cli.Do(ctx, http.MethodGet, "/api/records", nil)
		require.NoError(t, err)
		traceID := span.SpanContext().TraceID().String()
		require.Contains(t, captured.Header.Get("traceparent"), traceID)
		// The framework header names the same trace, borrowed from the span.
		require.Equal(t, traceID, captured.Header.Get(consts.HEADER_TRACE_ID))
	})

	t.Run("a context that belongs to no trace", func(t *testing.T) {
		_, err := cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
		require.NoError(t, err)
		require.Empty(t, captured.Header.Get(consts.HEADER_TRACE_ID))
		require.Empty(t, captured.Header.Get("traceparent"))
	})
}
