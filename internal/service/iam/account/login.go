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
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
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
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type LoginService struct {
	service.Base[*model.Empty, *modeliamaccount.LoginReq, *modeliamaccount.LoginRsp]
}

// Create authenticates an IAM account and creates a new session.
//
// The local login path verifies username, password, account status, and any
// required 2FA proof before creating the session. TOTP backup-code verification
// is delegated to the twofa service so recovery codes are consumed through the
// same one-time transactional path used by other twofa flows.
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

	// Check if user has 2FA enabled
	has2FA, err := checkUserHas2FA(ctx, user.ID)
	if err != nil {
		log.Errorz("failed to check 2FA status", zap.String("user_id", user.ID), zap.Error(err))
		return nil, errors.New("internal server error")
	}

	// If user has 2FA enabled, validate the 2FA code
	if has2FA {
		// Check if either TOTP code or backup code is provided
		if req.TOTPCode == "" && req.BackupCode == "" {
			log.Infoz("2FA required but no code provided", zap.String("username", req.Username))
			return nil, errors.New("2FA verification required")
		}

		// Validate TOTP code if provided
		if req.TOTPCode != "" {
			if err = validateTOTPCode(ctx, user.ID, req.TOTPCode); err != nil {
				log.Warnz("invalid TOTP code", zap.String("username", req.Username), zap.Error(err))
				return nil, errors.New("invalid 2FA code")
			}
			log.Infoz("TOTP code validated successfully", zap.String("username", req.Username))
		} else if req.BackupCode != "" {
			// Validate backup code if provided
			if err = servicetwofa.ConsumeTOTPBackupCode(ctx, user.ID, req.BackupCode); err != nil {
				log.Warnz("invalid backup code", zap.String("username", req.Username), zap.Error(err))
				return nil, errors.New("invalid backup code")
			}
			log.Infoz("backup code validated successfully", zap.String("username", req.Username))
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

// checkUserHas2FA checks if the user has active TOTP devices
func checkUserHas2FA(ctx *types.ServiceContext, userID string) (bool, error) {
	if !servicetwofa.Enabled {
		return false, nil
	}

	db := database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext())
	devices := make([]*modeltwofa.TOTPDevice, 0)

	if err := db.WithQuery(&modeltwofa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}).List(&devices); err != nil {
		return false, fmt.Errorf("failed to query TOTP devices: %w", err)
	}

	return len(devices) > 0, nil
}

// validateTOTPCode validates the provided TOTP code for the user
func validateTOTPCode(ctx *types.ServiceContext, userID, code string) error {
	if code == "" {
		return errors.New("TOTP code is required")
	}

	db := database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext())
	devices := make([]*modeltwofa.TOTPDevice, 0)

	if err := db.WithQuery(&modeltwofa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}).List(&devices); err != nil {
		return fmt.Errorf("failed to query TOTP devices: %w", err)
	}

	if len(devices) == 0 {
		return errors.New("no active TOTP devices found")
	}

	// Try to validate the code against all active devices
	for _, device := range devices {
		if totp.Validate(code, device.Secret) {
			return nil
		}
	}

	return errors.New("invalid TOTP code")
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
