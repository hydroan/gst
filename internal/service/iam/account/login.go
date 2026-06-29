package serviceiamaccount

import (
	// "fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	// modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	// servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	// servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
)

type LoginService struct {
	service.Base[*modeliamaccount.Login, *modeliamaccount.LoginReq, *modeliamaccount.LoginRsp]
}

// Create authenticates an IAM account and creates a new session.
//
// The local login path verifies username, password, and account status before
// creating the session.
func (s *LoginService) Create(ctx *types.ServiceContext, req *modeliamaccount.LoginReq) (rsp *modeliamaccount.LoginRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	// Validate input
	if req.Username == "" {
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	ua := useragent.New(ctx.UserAgent())
	engineName, _ := ua.Engine()
	browserName, _ := ua.Browser()

	// Logmgmt integration is disabled while IAM is decoupled from optional modules.
	//
	// var success bool
	// defer func() {
	// 	// Write failed login log.
	// 	if !success && servicelogmgmt.Enabled {
	// 		if logErr := database.Database[*modellogmgmt.LoginLog](ctx).Create(&modellogmgmt.LoginLog{
	// 			Username: req.Username,
	// 			ClientIP: ctx.ClientIP(),
	// 			Status:   modellogmgmt.LoginStatusFailure,
	// 			Source:   ctx.UserAgent(),
	// 			Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
	// 			Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
	// 			Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
	// 		}); logErr != nil {
	// 			log.Warnz("failed to write login log", zap.Error(logErr))
	// 		}
	// 	}
	// }()

	// Find user by username
	users := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&users); err != nil {
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
		return nil, service.NewError(http.StatusForbidden, "account disabled")
	}
	if user.Status == modeliamuser.UserStatusLocked {
		return nil, service.NewError(http.StatusForbidden, "account locked")
	}

	credential, err := LoadPasswordCredential(ctx, user.ID)
	if err != nil {
		log.Warnz("password credential not found", zap.String("username", req.Username), zap.Error(err))
		return nil, errors.New("invalid username or password")
	}
	if credential.LockedUntil != nil && credential.LockedUntil.After(time.Now()) {
		return nil, service.NewError(http.StatusForbidden, "account locked")
	}

	// Verify password
	if err = VerifyPasswordCredential(credential, req.Password); err != nil {
		log.Warnz("invalid password", zap.String("username", req.Username))
		return nil, errors.New("invalid username or password")
	}

	// MFA integration is disabled while IAM is decoupled from optional modules.
	//
	// if err = servicemfa.VerifyLoginSecondFactor(ctx, user.ID, servicemfa.LoginSecondFactor{
	// 	TOTPCode:   req.TOTPCode,
	// 	BackupCode: req.BackupCode,
	// }); err != nil {
	// 	switch {
	// 	case errors.Is(err, servicemfa.ErrLoginSecondFactorRequired):
	// 		log.Infoz("MFA required but no code provided", zap.String("username", req.Username))
	// 		return nil, errors.New("MFA verification required")
	// 	case errors.Is(err, servicemfa.ErrLoginSecondFactorConflict),
	// 		errors.Is(err, servicemfa.ErrLoginTOTPCodeInvalid):
	// 		log.Warnz("invalid TOTP code", zap.String("username", req.Username), zap.Error(err))
	// 		return nil, errors.New("invalid MFA code")
	// 	case errors.Is(err, servicemfa.ErrLoginBackupCodeInvalid):
	// 		log.Warnz("invalid backup code", zap.String("username", req.Username), zap.Error(err))
	// 		return nil, errors.New("invalid backup code")
	// 	default:
	// 		log.Errorz("failed to verify login MFA", zap.String("user_id", user.ID), zap.Error(err))
	// 		return nil, errors.New("internal server error")
	// 	}
	// }

	now := time.Now()
	// Create session
	sessionID, err := serviceiamsession.NewSessionID()
	if err != nil {
		log.Errorz("failed to create session id", zap.Error(err))
		return nil, errors.New("failed to create session id")
	}
	prefixedSessionID := modeliamsession.SessionIDKey(sessionID)
	expire := serviceiamsession.GetSessionExpiration()
	expiresAt := now.Add(expire)

	// Create session data for local user
	sessionData := modeliamsession.Session{
		ID:                 sessionID,
		UserID:             user.ID,
		Username:           user.Username,
		MustChangePassword: credential.MustChangePassword,
		ClientIP:           ctx.ClientIP(),
		UserAgent:          ctx.UserAgent(),
		OS:                 ua.OS(),
		Platform:           ua.Platform(),
		EngineName:         engineName,
		BrowserName:        browserName,
		Status:             modeliamsession.SessionStatusActive,
		IssuedAt:           now,
		LastSeenAt:         now,
		ExpiresAt:          expiresAt,
	}
	// Store session in Redis
	redisCache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	if err = redisCache.Set(prefixedSessionID, sessionData, expire); err != nil {
		log.Errorz("failed to set session in redis", zap.Error(err))
		return nil, errors.New("failed to set session in redis")
	}
	if err = serviceiamsession.IndexSession(ctx, sessionData); err != nil {
		_ = redisCache.Delete(prefixedSessionID)
		log.Errorz("failed to track user session in redis", zap.Error(err))
		return nil, errors.New("failed to track user session in redis")
	}

	credential.FailedLoginCount = 0
	if err = database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithoutHook().
		WithSelect("user_id", "failed_login_count").
		Update(credential); err != nil {
		log.Warnz("failed to update password credential statistics", zap.Error(err))
	}

	serviceiamsession.SessionManager.SetCookie(ctx, sessionID, expire)

	log.Infoz("user logged in successfully", zap.String("username", req.Username), zap.String("user_id", user.ID))

	// Logmgmt integration is disabled while IAM is decoupled from optional modules.
	//
	// success = true
	// if servicelogmgmt.Enabled {
	// 	if err = database.Database[*modellogmgmt.LoginLog](ctx).Create(&modellogmgmt.LoginLog{
	// 		UserID:   user.ID,
	// 		Username: user.Username,
	// 		ClientIP: ctx.ClientIP(),
	// 		Status:   modellogmgmt.LoginStatusSuccess,
	//
	// 		Source:   ctx.UserAgent(),
	// 		Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
	// 		Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
	// 		Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
	// 	}); err != nil {
	// 		log.Warnz("failed to write login log", zap.Error(err))
	// 	}
	// }

	email := ""
	emailIdentity, err := LoadEmailIdentity(ctx, user.ID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			log.Warnz("failed to load email identity for login response", zap.String("user_id", user.ID), zap.Error(err))
		}
	} else {
		email = emailIdentity.Email
	}

	return serviceiamsession.BuildAuthenticatedSessionRsp(sessionData, user, email, now), nil
}
