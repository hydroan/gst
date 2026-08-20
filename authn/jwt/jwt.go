package jwt

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
	"github.com/mssola/useragent"
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

var sessionCache *expirable.LRU[string, *model.Session]

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

func Init() error {
	return nil
}

// func Init() error {
// 	sessionCache = expirable.NewLRU(0, func(_ string, s *model.Session) {
// 		_ = database.Database[*model.Session](context.Background()).WithPurge().Delete(s)
// 	}, config.App.Auth.RefreshTokenExpireDuration)
// 	sessions := make([]*model.Session, 0)
// 	if err := database.Database[*model.Session](context.Background()).WithLimit(-1).List(&sessions); err != nil {
// 		return errors.Wrap(err, "failed to list sessions")
// 	}
// 	for _, session := range sessions {
// 		setSession(session.UserID, session)
// 	}
//
// 	return nil
// }

// GenTokens generates an access token and a refresh token.
func GenTokens(userID string, username string, session *model.Session) (aToken, rToken string, err error) {
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

	if session == nil {
		session = new(model.Session)
	}
	session.AccessToken = aToken
	session.RefreshToken = rToken
	session.UserID = userID
	session.Username = username
	// setToken(aToken, rToken, session)
	setSession(userID, session)

	return aToken, rToken, nil
}

func RevokeTokens(userID string) {
	removeSession(userID)
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
func RefreshTokens(accessToken, refreshToken string, session *model.Session) (newAccessToken, newRefreshToken string, err error) {
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

	return GenTokens(accessClaims.UserID, accessClaims.Username, session)
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

func Verify(claims *Claims, accessToken, userAgent string) error {
	if claims == nil {
		return errors.New("claims is nil")
	}
	if accessToken == config.App.Auth.NoneExpireToken {
		return nil
	}

	session, found := GetSession(claims.UserID)
	if !found {
		return errors.New("session not found")
	}
	if session.AccessToken != accessToken {
		return errors.New("access token not match")
	}

	ua := useragent.New(userAgent)
	engineName, _ := ua.Engine()
	browserName, _ := ua.Browser()

	if session.Platform != ua.Platform() {
		return errors.New("platform not match")
	}
	if session.OS != ua.OS() {
		return errors.New("os not match")
	}
	if session.EngineName != engineName {
		return errors.New("engine not match")
	}
	if session.BrowserName != browserName {
		return errors.New("browser not match")
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
