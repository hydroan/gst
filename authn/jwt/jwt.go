package jwt

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types/consts"
)

const (
	MinUserIDLength   = 1
	MinUsernameLength = 3
)

var (
	ErrInvalidToken        = errors.New("invalid token")
	ErrInvalidAccessToken  = errors.New("invalid access token")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrTokenExpired        = errors.New("token expired")
	ErrTokenMalformed      = errors.New("token malformed")
	ErrTokenNotValidYet    = errors.New("token not valid yet")
)

var (
	secret = []byte("defaultSecret")
	issuer = consts.FrameworkName
)

type Claims struct {
	UserID            string `json:"user_id,omitempty"`
	Username          string `json:"username,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Scope             string `json:"scope,omitempty"`

	// Standard Claims
	AuthTime *jwt.NumericDate `json:"auth_time"` // The time at which the JWT was issued.
	Typ      string           `json:"typ"`       // The media type of this complete JWT. eg: Bearer
	Azp      string           `json:"azp"`       // The authorized party to which the ID Token was issued.
	Sid      string           `json:"sid"`       // An identifier for a session at the relying party.
	Acr      string           `json:"acr"`       // Authentication Context Class. Learn more
	AtHash   string           `json:"at_hash"`   // Access Token hash value encoded in base64url format.

	jwt.RegisteredClaims
}

// GenTokens generates an access token and a refresh token.
// GenTokens issues an access token and a refresh token for a user.
//
// The tokens are self-contained: everything a later Verify needs is signed into
// the access token, and nothing about them is written down here. That is what
// makes them usable across processes without a shared store, and it is the
// whole point of choosing them over an opaque session id.
func GenTokens(userID string, username string) (aToken, rToken string, err error) {
	if len(userID) < MinUserIDLength || len(username) < MinUsernameLength {
		return "", "", errors.New("invalid user id or username")
	}

	if username == config.App.Auth.NoneExpireUsername {
		return config.App.Auth.NoneExpireToken, "", nil
	}
	if aToken, err = genAccessToken(userID, username); err != nil {
		return "", "", err
	}
	if rToken, err = genRefreshToken(userID); err != nil {
		return "", "", err
	}

	return aToken, rToken, nil
}

func genAccessToken(userID string, username string) (token string, err error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		ExpiresAt: jwt.NewNumericDate(now.Add(config.App.Auth.AccessTokenExpireDuration)), // expiration time
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		Issuer:    issuer, // issuer
		Subject:   userID,
	}
	// NewWithClaims builds a signing object using the given signing method,
	// SignedString signs it with the given secret and returns the fully encoded token string.
	if token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret); err != nil {
		return "", errors.Wrap(err, "failed to generate access token")
	}
	return token, nil
}

func genRefreshToken(userID string) (rToken string, err error) {
	now := time.Now()
	// a refresh token carries no custom data,
	// sign it with the given secret and return the fully encoded token string
	if rToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(now.Add(config.App.Auth.RefreshTokenExpireDuration)), // expiration time
		IssuedAt:  jwt.NewNumericDate(now),                                                 // issued at
		NotBefore: jwt.NewNumericDate(now),                                                 // valid from
		Issuer:    issuer,                                                                  // issuer
		Subject:   userID,
	}).SignedString(secret); err != nil {
		return "", errors.Wrap(err, "failed to generate refresh token")
	}
	return rToken, nil
}

// RefreshTokens issues a new access token from the given refresh token.
func RefreshTokens(accessToken, refreshToken string) (newAccessToken, newRefreshToken string, err error) {
	// verify refresh token
	refreshClaims := new(Claims)
	var token *jwt.Token
	if token, err = jwt.ParseWithClaims(refreshToken, refreshClaims, keyFunc); err != nil {
		return "", "", errors.Wrap(err, ErrInvalidRefreshToken.Error())
	}
	if !token.Valid {
		return "", "", ErrInvalidRefreshToken
	}
	if time.Now().After(refreshClaims.ExpiresAt.Time) {
		return "", "", ErrTokenExpired
	}

	// verify access token
	accessClaims := new(Claims)
	if token, err = jwt.ParseWithClaims(accessToken, accessClaims, keyFunc); err != nil {
		if !errors.Is(err, jwt.ErrTokenExpired) {
			return "", "", errors.Wrap(err, ErrInvalidAccessToken.Error())
		}
	} else if !token.Valid {
		return "", "", ErrInvalidAccessToken
	}
	// verify whether subject is the same
	if refreshClaims.Subject != accessClaims.Subject {
		return "", "", ErrTokenMalformed
	}

	return GenTokens(accessClaims.UserID, accessClaims.Username)
}

// ParseToken parse token
func ParseToken(tokenStr string) (*Claims, error) {
	if len(tokenStr) == 0 {
		return nil, ErrTokenMalformed
	}
	if tokenStr == config.App.Auth.NoneExpireToken {
		return &Claims{
			UserID: "root",
			// This must be either root or admin, but admin is reserved for regular
			// administration, so root is used here. It pairs with casbin.
			Username: "root",
			Issuer:   issuer, Subject: "root",
		}, nil
	}

	claims := new(Claims)
	token, err := jwt.ParseWithClaims(tokenStr, claims, keyFunc)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return nil, ErrTokenNotValidYet
		case errors.Is(err, jwt.ErrTokenMalformed):
			return nil, ErrTokenMalformed
		default:
			return nil, errors.Wrap(err, "failed to parse token")
		}
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Issuer != issuer {
		return nil, errors.New("invalid token issuer")
	}
	return claims, nil
}

// Verify checks an access token beyond the signature the parser already
// validated.
//
// It asserts nothing about where the token is being used from. The previous
// version compared the caller's browser and operating system against a stored
// session, which made a stateless token stateful and answered "not match" for
// a user who simply switched browsers; binding a token to a device is the job
// of whoever issues it, and requires a store this package deliberately has not.
func Verify(claims *Claims, accessToken string) error {
	if claims == nil {
		return errors.New("claims is nil")
	}
	if accessToken == config.App.Auth.NoneExpireToken {
		return nil
	}
	if len(claims.UserID) < MinUserIDLength || len(claims.Username) < MinUsernameLength {
		return ErrInvalidAccessToken
	}
	return nil
}

func ParseTokenFromHeader(header http.Header) (token string, claims *Claims, err error) {
	value := header.Get("Authorization")
	if len(value) == 0 {
		return "", nil, ErrInvalidToken
	}

	// split on the space
	items := strings.SplitN(value, " ", 2)
	if len(items) != 2 || items[0] != "Bearer" {
		return "", nil, ErrInvalidToken
	}
	token = items[1]
	claims, err = ParseToken(items[1])
	return token, claims, err
}
func keyFunc(token *jwt.Token) (any, error) { return secret, nil }

// Token is the OAuth 2.0 token response of RFC 6749 section 5.1, plus the
// id_token OpenID Connect adds to it.
//
// It describes what an authorization server hands back, so it is defined next
// to the code that issues and parses these tokens rather than next to a session
// — IAM's own sessions are an opaque cookie and carry none of this.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`

	// TokenType is the scheme the access token is presented with, "Bearer" for
	// everything this package issues.
	TokenType string `json:"token_type"`

	// ExpiresIn is the access token's remaining lifetime in seconds, counted
	// from when the response was produced.
	ExpiresIn int `json:"expires_in,omitempty"`

	// Scope is the granted scope, present only when it differs from what was
	// requested, as RFC 6749 requires.
	Scope string `json:"scope,omitempty"`
}
