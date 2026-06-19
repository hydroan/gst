package serviceiamaccount

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamgroup "github.com/hydroan/gst/internal/model/iam/group"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type LoginService struct {
	service.Base[*model.Empty, *modeliamaccount.LoginReq, *modeliamaccount.LoginRsp]
}

// Create authenticates an IAM account and creates a new session.
//
// The local login path verifies username, password, account status, and any
// required 2FA proof before creating the session. The twofa service owns the
// login second-factor decision, including disabled-module behavior, active
// device checks, TOTP validation, and recovery-code consumption.
func (s *LoginService) Create(ctx *types.ServiceContext, req *modeliamaccount.LoginReq) (rsp *modeliamaccount.LoginRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	// return keycloakLogin(ctx, log, req)
	return localLogin(ctx, log, req)
}

// localLogin performs username/password authentication and optional 2FA verification.
func localLogin(ctx *types.ServiceContext, log types.Logger, req *modeliamaccount.LoginReq) (rsp *modeliamaccount.LoginRsp, err error) {
	// Validate input
	if req.Username == "" {
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	var success bool
	ua := useragent.New(ctx.UserAgent)
	engineName, engineVersion := ua.Engine()
	browserName, browserVersion := ua.Browser()

	defer func() {
		// write login log.
		if !success && servicelogmgmt.Enabled {
			if logErr := database.Database[*modellogmgmt.LoginLog](ctx.DatabaseContext()).Create(&modellogmgmt.LoginLog{
				Username: req.Username,
				ClientIP: ctx.ClientIP,
				Status:   modellogmgmt.LoginStatusFailure,
				Source:   ctx.Request.UserAgent(),
				Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
				Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
				Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
			}); logErr != nil {
				log.Warnz("failed to write login log", zap.Error(logErr))
			}
		}
	}()

	// Find user by username
	users := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&users); err != nil {
		log.Errorz("failed to query user", zap.Error(err))
		return nil, errors.New("invalid username or password")
	}
	if len(users) == 0 {
		log.Warnz("user not found", zap.String("username", req.Username))
		return nil, errors.New("invalid username or password")
	}
	user := users[0]

	// Check if user is enabled
	if user.Status == modeliamuser.UserStatusInactive {
		return nil, types.NewServiceError(http.StatusForbidden, "", response.CodeAccountInactive)
	}
	if user.Status == modeliamuser.UserStatusLocked {
		return nil, types.NewServiceError(http.StatusForbidden, "", response.CodeAccountLocked)
	}

	// Verify password
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Warnz("invalid password", zap.String("username", req.Username))
		return nil, errors.New("invalid username or password")
	}

	if err = servicetwofa.VerifyLoginSecondFactor(ctx, user.ID, servicetwofa.LoginSecondFactor{
		TOTPCode:   req.TOTPCode,
		BackupCode: req.BackupCode,
	}); err != nil {
		switch {
		case errors.Is(err, servicetwofa.ErrLoginSecondFactorRequired):
			log.Infoz("2FA required but no code provided", zap.String("username", req.Username))
			return nil, errors.New("2FA verification required")
		case errors.Is(err, servicetwofa.ErrLoginSecondFactorConflict),
			errors.Is(err, servicetwofa.ErrLoginTOTPCodeInvalid):
			log.Warnz("invalid TOTP code", zap.String("username", req.Username), zap.Error(err))
			return nil, errors.New("invalid 2FA code")
		case errors.Is(err, servicetwofa.ErrLoginBackupCodeInvalid):
			log.Warnz("invalid backup code", zap.String("username", req.Username), zap.Error(err))
			return nil, errors.New("invalid backup code")
		default:
			log.Errorz("failed to verify login 2FA", zap.String("user_id", user.ID), zap.Error(err))
			return nil, errors.New("internal server error")
		}
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).Update(user); err != nil {
		log.Errorz("failed to update last login time", zap.Error(err))
		// Don't fail the login for this
	}

	// Query the group of the user
	group := new(modeliamgroup.Group)
	_ = database.Database[*modeliamgroup.Group](ctx.DatabaseContext()).Get(group, user.GroupID)

	// Parse user agent for session info

	// Create session
	sessionID := util.UUID()
	prefixedSessionID := modeliamsession.SessionIDKey(sessionID)
	expire := serviceiamsession.GetSessionExpiration()
	expiresAt := now.Add(expire)

	// Create session data for local user
	sessionData := modeliamsession.Session{
		ID:                 sessionID,
		UserID:             user.ID,
		Username:           user.Username,
		Email:              util.Deref(user.Email),
		Status:             string(user.Status),
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		GroupID:            user.GroupID,
		GroupName:          group.Name,
		MustChangePassword: user.MustChangePassword,
		ClientIP:           ctx.ClientIP,
		UserAgent:          ctx.Request.UserAgent(),
		OS:                 ua.OS(),
		Platform:           ua.Platform(),
		EngineName:         engineName,
		BrowserName:        browserName,
		State:              modeliamsession.SessionStatusActive,
		IssuedAt:           now,
		LastSeenAt:         now,
		ExpiresAt:          expiresAt,
	}
	// Store session in Redis
	if err = redis.Cache[modeliamsession.Session]().Set(prefixedSessionID, sessionData, expire); err != nil {
		log.Errorz("failed to set session in redis", zap.Error(err))
		return nil, errors.New("failed to set session in redis")
	}
	if err = serviceiamsession.TrackUserSession(sessionData); err != nil {
		log.Errorz("failed to track user session in redis", zap.Error(err))
		return nil, errors.New("failed to track user session in redis")
	}

	// Set cookie
	//nolint:gosec // Secure is intentionally false so local HTTP development keeps session cookies.
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(expire.Seconds()), // Use configured expiration time
		HttpOnly: true,                  // More secure
		Secure:   false,                 // Set to false for local development
		SameSite: http.SameSiteLaxMode,  // Lax mode
	})

	log.Infoz("user logged in successfully", zap.String("username", req.Username), zap.String("user_id", user.ID))

	// write login log
	success = true
	if servicelogmgmt.Enabled {
		if err = database.Database[*modellogmgmt.LoginLog](ctx.DatabaseContext()).Create(&modellogmgmt.LoginLog{
			UserID:   user.ID,
			Username: user.Username,
			ClientIP: ctx.ClientIP,
			Status:   modellogmgmt.LoginStatusSuccess,

			Source:   ctx.Request.UserAgent(),
			Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
			Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
			Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
		}); err != nil {
			log.Warnz("failed to write login log", zap.Error(err))
		}
	}

	return &modeliamaccount.LoginRsp{
		SessionID: sessionID,
	}, nil
}

// func keycloakLogin(ctx *types.ServiceContext, log types.Logger, req *iam.LoginReq) (rsp *iam.LoginRsp, err error) {
// 	kccfg := config.Get[configx.Keycloak]()
//
// 	// keycloak 校验用户名和密码
// 	tokens, err := keycloak.IdentityLogin(log, req.Username, req.Password)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 获取用户信息
// 	userInfo, err := keycloak.UserInfo(log, tokens.AccessToken)
// 	if err != nil {
// 		log.Error(err)
// 		return nil, err
// 	}
//
// 	// 解析 token、解析前端浏览器信息
// 	jwksURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", kccfg.Addr, kccfg.Realm)
// 	claims := &helper.Claims{}
// 	if _, err := jwt.ParseWithClaims(tokens.AccessToken, claims, helper.KeyFuncForKeycloak(jwksURL)); err != nil {
// 		log.Error(err)
// 		return nil, err
// 	}
// 	ua := useragent.New(ctx.UserAgent)
// 	engineName, _ := ua.Engine()
// 	browserName, _ := ua.Browser()
//
// 	// 存入 redis
// 	sessionID := util.UUID()
// 	redisKey := modeliamsession.SessionIDKey(sessionID)
// 	if err := redis.Cache[iam.Session]().Set(redisKey, iam.Session{
// 		UserID:      claims.Sub,
// 		Username:    claims.PreferredUsername,
// 		Email:       claims.Email,
// 		OS:          ua.OS(),
// 		Platform:    ua.Platform(),
// 		EngineName:  engineName,
// 		BrowserName: browserName,
// 		Token:       *tokens,
// 		UserInfo:    userInfo,
// 	}, 8*time.Hour); err != nil {
// 		log.Error("failed to set session in redis", zap.Error(err))
// 		return nil, fmt.Errorf("failed to set session in redis")
// 	}
//
// 	// 设置前端 cookie
// 	http.SetCookie(ctx.Writer, &http.Cookie{
// 		Name:  "session_id",
// 		Value: sessionID,
// 		Path:  "/",
// 		// MaxAge:   tokens.RefreshExpiresIn,
// 		MaxAge:   8 * 60 * 60,          // 8 hours
// 		HttpOnly: true,                 // 建议设为 true，更安全
// 		Secure:   false,                // 本地开发设为 false
// 		SameSite: http.SameSiteLaxMode, // 改为 Lax
// 	})
//
// 	// 返回给前端
// 	rsp = &iam.LoginRsp{
// 		SessionID: sessionID,
// 	}
//
// 	return rsp, nil
// }
