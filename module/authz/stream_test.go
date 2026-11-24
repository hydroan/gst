package authz_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/internal/service"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestAuthorizationGrantsAStreamAlikeOnBothListeners pins that a Stream
// action is one thing to grant, however it is reached: the WebSocket
// connection it is over HTTP and the streaming rpc it is over gRPC are both
// judged by the STREAM word on the route, the way the route list names the
// action, so a policy written from the list opens both. An HTTP method
// grants it on neither, the GET its connection is opened with included, and
// however the handshake is spelled: the decision follows what the route
// serves, not what the request says of itself.
func TestAuthorizationGrantsAStreamAlikeOnBothListeners(t *testing.T) {
	conn := grpcAuthzProbe(t)

	t.Run("the STREAM word grants the stream on both listeners", func(t *testing.T) {
		tailer := newStreamTailer(t, "stream_word", apiaction.MethodStream)

		stream, err := tailer.client.ServerStream[probeTail](t.Context(), probeTailPath, nil)
		require.NoError(t, err)
		defer stream.Close()
		tail, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, "42", tail.Thing, "the connection reached the action of the route")

		require.NoError(t, authzProbeStream(t, conn, tailer.grpcSessionID))
	})

	t.Run("an HTTP method grants it on neither", func(t *testing.T) {
		tailer := newStreamTailer(t, "stream_get", http.MethodGet)

		_, err := tailer.client.ServerStream[probeTail](t.Context(), probeTailPath, nil)
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")

		require.Equal(t, codes.PermissionDenied, status.Code(authzProbeStream(t, conn, tailer.grpcSessionID)))
	})

	t.Run("a handshake spelled over two Connection headers is judged alike", func(t *testing.T) {
		tailer := newStreamTailer(t, "stream_split", http.MethodGet)

		require.Equal(t, http.StatusForbidden, splitHandshakeStatus(t, probeTailPath, tailer.sessionID))
	})

	t.Run("the route list names the stream by the STREAM word", func(t *testing.T) {
		rsp, err := authzSessionClient(t, authzAdminSessionID(t)).Get[authz.RoutesRsp](t.Context(), routesPath)
		require.NoError(t, err)
		requireRoute(t, rsp.Items, "/api/probe-things/{thing}/tail", []string{apiaction.MethodStream})
	})
}

// probeTailPath is a concrete path of probeTailRoute, the route the probe's
// Tail rpc is described with and the stream below is served at over HTTP.
const probeTailPath = "/api/probe-things/42/tail"

// streamUserAgent is the device the sessions of the stream cases are
// established and presented from: the handshake written by hand presents it
// too, a session being held to the device it was established on.
const streamUserAgent = "gst-authz-stream-test"

// probeTail is the message the probe's stream carries over HTTP.
type probeTail struct {
	Thing string `json:"thing"`
}

// probeTailer serves the probe's Stream action over HTTP: one message naming
// the thing of the route, then the end of the stream.
type probeTailer struct {
	service.Base[*model.Empty, *model.Empty, *probeTail]
}

func (s *probeTailer) Stream(ctx *types.ServiceContext, _ *model.Empty, stream *types.ServerStream[*probeTail]) error {
	return stream.Send(&probeTail{Thing: ctx.Param("thing")})
}

// The service registers in init and the route after the bootstrap, the split
// generated code has; TestMain hands registerProbeStream to the server as its
// Routes.
func init() {
	service.RegisterInstance[*model.Empty, *model.Empty, *probeTail](apiaction.Stream, "probe-things/:thing/tail", &probeTailer{})
}

// registerProbeStream serves the probe's Stream action over HTTP at
// probeTailRoute, behind the session check and the authorization.
func registerProbeStream() error {
	router.Register[*model.Empty, *model.Empty, *probeTail](
		router.Auth(), "probe-things/:thing/tail", &types.ControllerConfig[*model.Empty]{}, apiaction.Stream,
	)
	return nil
}

// streamTailer is a user whose role is granted one action on the route of
// the probe's stream, with its session on each listener.
type streamTailer struct {
	sessionID     string
	client        *client.Client
	grpcSessionID string
}

// newStreamTailer signs a user up, binds it a role granted action on the
// route of the probe's stream, written the way the route list spells it, and
// logs it in once for each listener.
func newStreamTailer(t *testing.T, name, action string) streamTailer {
	t.Helper()

	username := authzTestUsername(name)
	userID, sessionID := authzSignupAndLoginUserWithUserAgent(t, username, "12345678", streamUserAgent)
	roleID := newAuthorizationRole(t, authorizationSubject{userID: userID}, name)
	authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{Object: "/api/probe-things/{thing}/tail", Action: action})

	cli, err := client.New(baseURL, client.WithUserAgent(streamUserAgent), client.WithCookie(&http.Cookie{Name: "session_id", Value: sessionID}))
	require.NoError(t, err)
	return streamTailer{
		sessionID:     sessionID,
		client:        cli,
		grpcSessionID: loginSessionIDFromCookieWithUserAgent(t, iam.LoginReq{Username: username, Password: "12345678"}, grpcUserAgent),
	}
}

// splitHandshakeStatus writes the WebSocket handshake of path by hand, its
// Connection header split over two lines with the upgrade on the second,
// which the WebSocket library reads as a handshake all the same, and returns
// the status the server answers it with.
func splitHandshakeStatus(t *testing.T, path, sessionID string) int {
	t.Helper()

	target, err := url.Parse(baseURL)
	require.NoError(t, err)
	conn, err := net.DialTimeout("tcp", target.Host, 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))

	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nCookie: session_id=%s\r\n"+
		"Connection: keep-alive\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n"+
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
		path, target.Host, streamUserAgent, sessionID)
	require.NoError(t, err)
	rsp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer rsp.Body.Close()
	return rsp.StatusCode
}
