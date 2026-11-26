package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// deadlineUnderTest is short enough to keep the tests fast and long enough
// for a handler that answers at once to finish well inside it.
const deadlineUnderTest = 30 * time.Millisecond

func TestDeadlineLeavesAStreamingRouteAlone(t *testing.T) {
	MarkStreamingRoute(http.MethodGet, "/api/deadline/stream")

	router := newDeadlineRouter()
	router.GET("/api/deadline/stream", func(c *gin.Context) {
		// Outlives the deadline on purpose; only the streaming exemption lets
		// this response finish.
		time.Sleep(4 * deadlineUnderTest)
		c.String(http.StatusOK, "finished")
	})

	stream := serveDeadline(router, "/api/deadline/stream")
	require.Equal(t, http.StatusOK, stream.Code)
	require.Equal(t, "finished", stream.Body.String())
}

func TestDeadlinePassesAResponseStartedInTime(t *testing.T) {
	router := newDeadlineRouter()
	router.GET("/api/deadline/prompt", func(c *gin.Context) {
		c.Header("X-Handler", "set")
		c.String(http.StatusCreated, "answered")
	})

	w := serveDeadline(router, "/api/deadline/prompt")
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "answered", w.Body.String())
	require.Equal(t, "set", w.Header().Get("X-Handler"))
	require.Equal(t, "kept", w.Header().Get("X-Upstream"))
}

// TestDeadlineAnswersALateResponseWithGatewayTimeout covers a handler that
// ignores the deadline and only starts its response after it: the client gets
// the 504 envelope instead, and nothing the handler set or wrote gets through.
// The test waits for the handler before looking, so a write that lands after
// the middleware has returned would show up here, and under -race so would the
// handler touching the context the engine has already taken back.
func TestDeadlineAnswersALateResponseWithGatewayTimeout(t *testing.T) {
	router := newDeadlineRouter()
	finished := make(chan struct{})
	router.GET("/api/deadline/late", func(c *gin.Context) {
		defer close(finished)
		time.Sleep(4 * deadlineUnderTest)
		c.Header("X-Handler", "set")
		c.String(http.StatusOK, "late")
	})

	w := serveDeadline(router, "/api/deadline/late")
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not finish")
	}

	requireGatewayTimeout(t, w)
	require.Empty(t, w.Header().Get("X-Handler"))
	require.Equal(t, "kept", w.Header().Get("X-Upstream"))
}

// TestDeadlineAnswersAStoppedHandlerWithGatewayTimeout covers a handler that
// honors the deadline: it stops when the request context ends and writes an
// error of its own, which the 504 envelope replaces.
func TestDeadlineAnswersAStoppedHandlerWithGatewayTimeout(t *testing.T) {
	router := newDeadlineRouter()
	router.GET("/api/deadline/stopped", func(c *gin.Context) {
		<-c.Request.Context().Done()
		c.JSON(http.StatusInternalServerError, gin.H{"error": c.Request.Context().Err().Error()})
	})

	requireGatewayTimeout(t, serveDeadline(router, "/api/deadline/stopped"))
}

func TestDeadlineHandsAPanicToRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery(), deadline(deadlineUnderTest))
	router.GET("/api/deadline/panic", func(*gin.Context) { panic("handler failed") })

	w := serveDeadline(router, "/api/deadline/panic")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestDeadlineSettlesTheResponseOnTheCallThatStartsIt covers the calls other
// than Write that start a response — WriteString, WriteHeaderNow, Flush and
// Hijack: made before the deadline they go through, and made after it they
// are dropped for the 504 envelope, a late WriteString and Hijack telling
// the handler with http.ErrHandlerTimeout. A Hijack in time takes a
// connection, which TestDeadlineHandsOverAConnectionHijackedInTime covers.
func TestDeadlineSettlesTheResponseOnTheCallThatStartsIt(t *testing.T) {
	for _, tt := range []struct {
		name    string
		start   func(c *gin.Context) error
		lateErr error
		code    int
		body    string
	}{
		{name: "WriteString", start: func(c *gin.Context) error {
			_, err := c.Writer.WriteString("started")
			return err
		}, lateErr: http.ErrHandlerTimeout, code: http.StatusOK, body: "started"},
		{name: "WriteHeaderNow", start: func(c *gin.Context) error {
			c.Status(http.StatusAccepted)
			c.Writer.WriteHeaderNow()
			return nil
		}, code: http.StatusAccepted},
		{name: "Flush", start: func(c *gin.Context) error {
			c.Status(http.StatusAccepted)
			c.Writer.Flush()
			return nil
		}, code: http.StatusAccepted},
		{name: "Hijack", start: func(c *gin.Context) error {
			_, _, err := c.Writer.Hijack()
			return err
		}, lateErr: http.ErrHandlerTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != 0 {
				router := newDeadlineRouter()
				router.GET("/api/deadline/start", func(c *gin.Context) { require.NoError(t, tt.start(c)) })
				w := serveDeadline(router, "/api/deadline/start")
				require.Equal(t, tt.code, w.Code)
				require.Equal(t, tt.body, w.Body.String())
				require.Equal(t, tt.name == "Flush", w.Flushed)
			}

			router := newDeadlineRouter()
			var err error
			router.GET("/api/deadline/late-start", func(c *gin.Context) {
				time.Sleep(4 * deadlineUnderTest)
				err = tt.start(c)
			})
			w := serveDeadline(router, "/api/deadline/late-start")
			requireGatewayTimeout(t, w)
			require.False(t, w.Flushed)
			require.ErrorIs(t, err, tt.lateErr)
		})
	}
}

// TestDeadlineHandsOverAConnectionHijackedInTime covers a handler taking over
// the connection before the deadline, the way a WebSocket upgrade does: it
// gets the connection and its answer on it reaches the client.
func TestDeadlineHandsOverAConnectionHijackedInTime(t *testing.T) {
	router := newDeadlineRouter()
	router.GET("/api/deadline/hijack", func(c *gin.Context) {
		conn, rw, err := c.Writer.Hijack()
		require.NoError(t, err)
		defer conn.Close()
		_, err = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nhijacked")
		require.NoError(t, err)
		require.NoError(t, rw.Flush())
	})
	server := httptest.NewServer(router)
	defer server.Close()

	rsp, err := http.Get(server.URL + "/api/deadline/hijack")
	require.NoError(t, err)
	defer rsp.Body.Close()
	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
	require.Equal(t, "hijacked", string(body))
}

// TestBuiltinHoldsARequestToTheWriteTimeoutOfTheServer pins where the chain
// takes its deadline from and where it mounts it: a request is held to
// server.write_timeout, a handler answering later than that is answered 504,
// and the 504 keeps what the chain set on the response ahead of the
// deadline, the CORS header a browser needs to read the answer among it. A
// write timeout of zero holds a request to no time at all.
func TestBuiltinHoldsARequestToTheWriteTimeoutOfTheServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalLogger := logger.HTTP
	logger.HTTP = zap.NewNop()
	t.Cleanup(func() { logger.HTTP = originalLogger })
	standInRequestMetrics(t)
	originalNames := RouteParamNames
	RouteParamNames = NewRouteParamsManager()
	t.Cleanup(func() { RouteParamNames = originalNames })
	originalTimeout := config.App.Server.WriteTimeout
	t.Cleanup(func() { config.App.Server.WriteTimeout = originalTimeout })

	serve := func(timeout time.Duration) *httptest.ResponseRecorder {
		config.App.Server.WriteTimeout = timeout
		router := gin.New()
		router.Use(Builtin()...)
		router.GET("/api/deadline/builtin", func(c *gin.Context) {
			time.Sleep(4 * deadlineUnderTest)
			c.String(http.StatusOK, "late")
		})
		req := httptest.NewRequest(http.MethodGet, "/api/deadline/builtin", nil)
		req.Header.Set("Origin", "https://app.example.com")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	held := serve(deadlineUnderTest)
	requireGatewayTimeout(t, held)
	require.Equal(t, "https://app.example.com", held.Header().Get("Access-Control-Allow-Origin"))

	free := serve(0)
	require.Equal(t, http.StatusOK, free.Code)
	require.Equal(t, "late", free.Body.String())
}

// newDeadlineRouter mounts the deadline behind a middleware that sets a
// header of its own, which a dropped late response must not take down with
// it.
func newDeadlineRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("X-Upstream", "kept")
		c.Next()
	}, deadline(deadlineUnderTest))
	return router
}

func serveDeadline(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// requireGatewayTimeout asserts the 504 envelope the deadline answers with.
func requireGatewayTimeout(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusGatewayTimeout, w.Code)
	var envelope struct {
		Msg string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
	require.Equal(t, "request timeout", envelope.Msg)
}
