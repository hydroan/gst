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
// 		_ = database.Database[*model.Session](nil).WithPurge().Delete(s)
// 	}, config.App.Auth.RefreshTokenExpireDuration)
// 	sessions := make([]*model.Session, 0)
// 	if err := database.Database[*model.Session](nil).WithLimit(-1).List(&sessions); err != nil {
// 		return errors.Wrap(err, "failed to list sessions")
// 	}
// 	for _, session := range sessions {
// 		setSession(session.UserID, session)
// 	}
//
// 	return nil
// }

// GenTokens 生成 access token 和 refresh token
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
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(config.App.Auth.AccessTokenExpireDuration)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    issuer, // 签发人
			Subject:   userID,
		},
	}
	// NewWithClaims 使用指定的签名方法创建签名对象
	// SignedString 使用指定的 secret 签名并获得完整的编码后的字符串 token
	if token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret); err != nil {
		return "", errors.Wrap(err, "failed to generate access token")
	}
	return token, nil
}

func genRefreshToken(userID string) (rToken string, err error) {
	now := time.Now()
	// refresh token 不需要任何自定义数据
	// 使用指定的 secret 签名并获得完整的编码后的字符串 token
	if rToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(now.Add(config.App.Auth.RefreshTokenExpireDuration)), // 过期时间
		IssuedAt:  jwt.NewNumericDate(now),                                                 // 签发时间
		NotBefore: jwt.NewNumericDate(now),                                                 // 生效时间
		Issuer:    issuer,                                                                  // 签发人
		Subject:   userID,
	}).SignedString(secret); err != nil {
		return "", errors.Wrap(err, "failed to generate refresh token")
	}
	return rToken, nil
}

// RefreshTokens 通过 refresh token 刷新一个新的 AccessToken
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
			// 这里必须写成 root 或者 admin, 但是 admin 需要作为普通管理使用,所以这里使用 root
			// 配合 casbin 使用.
			Username:         "root",
			RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "root"},
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

	// 按空格分割
	items := strings.SplitN(value, " ", 2)
	if len(items) != 2 || items[0] != "Bearer" {
		return "", nil, ErrInvalidToken
	}
	token = items[1]
	claims, err = ParseToken(items[1])
	return token, claims, err
}
func keyFunc(token *jwt.Token) (any, error) { return secret, nil }
