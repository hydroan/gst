package interceptor_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/interceptor"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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

// TestJwtAuthAdmitsABearerTokenAndNamesTheCaller pins the admitting path: a
// call carrying a token the framework issued, as "authorization: Bearer
// <token>", goes on with the token's user as the caller in the request
// metadata and the x-session-id metadata as the session, the fields the
// HTTP middleware sets from the same token.
func TestJwtAuthAdmitsABearerTokenAndNamesTheCaller(t *testing.T) {
	withSigningKey(t, "test-signing-key")
	token, _, err := jwt.GenTokens("u-1", "alice")
	require.NoError(t, err)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token, "x-session-id", "s-1"))

	ctx, err = interceptor.JwtAuth()(ctx)

	require.NoError(t, err)
	seen := requestctx.FromContext(ctx)
	require.Equal(t, "alice", seen.Username())
	require.Equal(t, "u-1", seen.UserID())
	require.Equal(t, "s-1", seen.SessionID())
}

// TestJwtAuthRefusesWhatItCannotVerify pins the refusing path: no
// authorization metadata, a scheme other than Bearer, a token that does not
// parse and one signed with another key are all answered Unauthenticated
// with one fixed message and no context to go on with; the reasons stay in
// the log, the way the HTTP middleware keeps them from a caller probing the
// checks.
func TestJwtAuthRefusesWhatItCannotVerify(t *testing.T) {
	withSigningKey(t, "another-signing-key")
	foreign, _, err := jwt.GenTokens("u-1", "alice")
	require.NoError(t, err)
	withSigningKey(t, "test-signing-key")
	for name, md := range map[string]metadata.MD{
		"without authorization":            {},
		"with another scheme":              metadata.Pairs("authorization", "Basic YWxpY2U6cGFzcw=="),
		"with a token that does not parse": metadata.Pairs("authorization", "Bearer not-a-token"),
		"with a token signed elsewhere":    metadata.Pairs("authorization", "Bearer "+foreign),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), md)

			admitted, err := interceptor.JwtAuth()(ctx)

			require.Equal(t, codes.Unauthenticated, status.Code(err))
			require.Equal(t, "invalid token", status.Convert(err).Message())
			require.Nil(t, admitted)
		})
	}
}
