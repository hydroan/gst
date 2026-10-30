package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// TestJwtAuthAdmitsABearerTokenAndNamesTheCaller pins the admitting path: a
// request carrying a token the framework issued, as "Authorization: Bearer
// <token>", goes on with the token's user and username in the request
// context and the X-Session-Id header as the session, the fields the gRPC
// interceptor sets from the same token.
func TestJwtAuthAdmitsABearerTokenAndNamesTheCaller(t *testing.T) {
	withSigningKey(t, "test-signing-key")
	token, _, err := jwt.GenTokens("u-1", "alice")
	require.NoError(t, err)

	w := serveJwtAuth(t, http.Header{"Authorization": {"Bearer " + token}, "X-Session-Id": {"s-1"}})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "u-1 alice s-1", w.Body.String())
}

// TestJwtAuthRefusesWhatItCannotVerify pins the refusing path: no
// Authorization header, a scheme other than Bearer, a token that does not
// parse and one signed with another key are all answered 401 with one fixed
// message, and the handler behind the middleware never runs; the reasons
// stay in the log, the way the gRPC interceptor keeps them from a caller
// probing the checks.
func TestJwtAuthRefusesWhatItCannotVerify(t *testing.T) {
	withSigningKey(t, "another-signing-key")
	foreign, _, err := jwt.GenTokens("u-1", "alice")
	require.NoError(t, err)
	withSigningKey(t, "test-signing-key")
	for name, header := range map[string]http.Header{
		"without authorization":            {},
		"with another scheme":              {"Authorization": {"Basic YWxpY2U6cGFzcw=="}},
		"with a token that does not parse": {"Authorization": {"Bearer not-a-token"}},
		"with a token signed elsewhere":    {"Authorization": {"Bearer " + foreign}},
	} {
		t.Run(name, func(t *testing.T) {
			w := serveJwtAuth(t, header)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			var envelope struct {
				Msg string `json:"msg"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
			require.Equal(t, "invalid token", envelope.Msg)
		})
	}
}

// withSigningKey configures the key the jwt package signs and verifies
// tokens with, and a lifetime for the tokens it issues, until the test
// ends.
func withSigningKey(t *testing.T, key string) {
	t.Helper()
	saved := config.App.Auth
	config.App.Auth.JWTSecret = key
	config.App.Auth.AccessTokenExpireDuration = time.Hour
	config.App.Auth.RefreshTokenExpireDuration = time.Hour
	t.Cleanup(func() { config.App.Auth = saved })
}

// serveJwtAuth sends a request with header through JwtAuth to a handler that
// answers 200 with the user, username and session the middleware put in the
// request context.
func serveJwtAuth(t *testing.T, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.JwtAuth())
	router.GET("/api/jwt", func(c *gin.Context) {
		c.String(http.StatusOK, "%s %s %s", c.GetString(requestctx.KeyUserID), c.GetString(requestctx.KeyUsername), c.GetString(requestctx.KeySessionID))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/jwt", nil)
	req.Header = header
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
