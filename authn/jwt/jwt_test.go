package jwt_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

const (
	sampleUserID   = "user-1"
	sampleUsername = "sample"
)

// withTokenLifetimes configures the durations token generation reads, which are
// zero in a bare test process and would otherwise issue tokens already expired.
func withTokenLifetimes(t *testing.T, access, refresh time.Duration) {
	t.Helper()

	previous := config.App.Auth
	t.Cleanup(func() { config.App.Auth = previous })
	config.App.Auth.AccessTokenExpireDuration = access
	config.App.Auth.RefreshTokenExpireDuration = refresh
}

func TestGenTokens(t *testing.T) {
	t.Run("issues_a_parsable_access_token", func(t *testing.T) {
		withTokenLifetimes(t, time.Hour, 24*time.Hour)

		accessToken, refreshToken, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)
		require.NotEmpty(t, accessToken)
		require.NotEmpty(t, refreshToken)

		claims, err := jwt.ParseToken(accessToken)
		require.NoError(t, err)
		require.Equal(t, sampleUserID, claims.UserID)
		require.Equal(t, sampleUsername, claims.Username)
	})

	// The tokens carry everything a later check needs, so two calls are
	// independent: issuing again does not disturb what was issued before, which
	// is the property that lets them work without a shared store.
	t.Run("does_not_invalidate_a_token_it_issued_earlier", func(t *testing.T) {
		withTokenLifetimes(t, time.Hour, 24*time.Hour)

		first, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)
		_, _, err = jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		claims, err := jwt.ParseToken(first)
		require.NoError(t, err)
		require.NoError(t, jwt.Verify(claims, first))
	})

	t.Run("rejects_a_user_it_cannot_name", func(t *testing.T) {
		withTokenLifetimes(t, time.Hour, 24*time.Hour)

		_, _, err := jwt.GenTokens("", sampleUsername)
		require.Error(t, err)
	})
}

func TestParseToken(t *testing.T) {
	withTokenLifetimes(t, time.Hour, 24*time.Hour)

	t.Run("rejects_an_empty_token", func(t *testing.T) {
		_, err := jwt.ParseToken("")
		require.ErrorIs(t, err, jwt.ErrTokenMalformed)
	})

	t.Run("rejects_a_token_that_is_not_a_token", func(t *testing.T) {
		_, err := jwt.ParseToken("not.a.token")
		require.Error(t, err)
	})

	t.Run("rejects_a_tampered_token", func(t *testing.T) {
		accessToken, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		// Flipping the last character breaks the signature without touching the
		// claims, which is the forgery the signature exists to catch.
		tampered := accessToken[:len(accessToken)-1] + "x"
		_, err = jwt.ParseToken(tampered)
		require.Error(t, err)
	})

	t.Run("rejects_an_expired_token", func(t *testing.T) {
		withTokenLifetimes(t, -time.Minute, 24*time.Hour)

		accessToken, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		_, err = jwt.ParseToken(accessToken)
		require.ErrorIs(t, err, jwt.ErrTokenExpired)
	})
}

func TestParseTokenFromHeader(t *testing.T) {
	withTokenLifetimes(t, time.Hour, 24*time.Hour)

	accessToken, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
	require.NoError(t, err)

	t.Run("reads_a_bearer_token", func(t *testing.T) {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+accessToken)

		token, claims, err := jwt.ParseTokenFromHeader(header)
		require.NoError(t, err)
		require.Equal(t, accessToken, token)
		require.Equal(t, sampleUserID, claims.UserID)
	})

	for name, value := range map[string]string{
		"rejects_a_missing_header": "",
		"rejects_another_scheme":   "Basic " + accessToken,
		"rejects_a_bare_token":     accessToken,
	} {
		t.Run(name, func(t *testing.T) {
			header := http.Header{}
			if value != "" {
				header.Set("Authorization", value)
			}

			_, _, err := jwt.ParseTokenFromHeader(header)
			require.ErrorIs(t, err, jwt.ErrInvalidToken)
		})
	}
}

// TestVerify covers what a token is checked for once its signature has already
// been validated by the parser.
//
// Nothing about the caller's device takes part. An earlier version compared the
// request's browser and operating system against a stored session, which made a
// stateless token stateful and refused a user who had merely switched browsers.
func TestVerify(t *testing.T) {
	withTokenLifetimes(t, time.Hour, 24*time.Hour)

	t.Run("accepts_a_token_it_issued", func(t *testing.T) {
		accessToken, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		claims, err := jwt.ParseToken(accessToken)
		require.NoError(t, err)
		require.NoError(t, jwt.Verify(claims, accessToken))
	})

	t.Run("rejects_claims_that_name_nobody", func(t *testing.T) {
		require.Error(t, jwt.Verify(&jwt.Claims{}, "any-token"))
	})

	t.Run("rejects_nil_claims", func(t *testing.T) {
		require.Error(t, jwt.Verify(nil, "any-token"))
	})
}

func TestRefreshTokens(t *testing.T) {
	withTokenLifetimes(t, time.Hour, 24*time.Hour)

	t.Run("issues_a_new_pair", func(t *testing.T) {
		accessToken, refreshToken, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		newAccess, newRefresh, err := jwt.RefreshTokens(accessToken, refreshToken)
		require.NoError(t, err)
		require.NotEmpty(t, newAccess)
		require.NotEmpty(t, newRefresh)

		claims, err := jwt.ParseToken(newAccess)
		require.NoError(t, err)
		require.Equal(t, sampleUserID, claims.UserID)
	})

	t.Run("rejects_a_refresh_token_that_is_not_one", func(t *testing.T) {
		accessToken, _, err := jwt.GenTokens(sampleUserID, sampleUsername)
		require.NoError(t, err)

		_, _, err = jwt.RefreshTokens(accessToken, "not-a-refresh-token")
		require.Error(t, err)
	})
}
