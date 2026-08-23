package serviceiamaccount

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
func (l *LoginService) Create(ctx *types.ServiceContext, req *modeliamaccount.LoginReq) (rsp *modeliamaccount.LoginRsp, err error) {
	log := l.WithContext(ctx, ctx.Phase())
	// Validate input
	if req.Username == "" {
		return nil, service.NewError(http.StatusBadRequest, "username is required")
	}
	if req.Password == "" {
		return nil, service.NewError(http.StatusBadRequest, "password is required")
	}

	ua := useragent.New(ctx.UserAgent())
	engineName, engineVersion := ua.Engine()
	browserName, browserVersion := ua.Browser()

	// Login observers hear the settled outcome of every attempt that passed
	// input validation, no matter which branch below rejected it.
	var targetUser *modeliamuser.User
	defer func() {
		event := authn.LoginEvent{
			Kind:           authn.LoginEventFailed,
			Username:       req.Username,
			TenantID:       strings.TrimSpace(req.TenantID),
			ClientIP:       ctx.ClientIP(),
			UserAgent:      ctx.UserAgent(),
			OS:             ua.OS(),
			Platform:       ua.Platform(),
			EngineName:     engineName,
			EngineVersion:  engineVersion,
			BrowserName:    browserName,
			BrowserVersion: browserVersion,
			At:             time.Now().UTC(),
		}
		if err == nil {
			event.Kind = authn.LoginEventSucceeded
		}
		if targetUser != nil {
			event.UserID = targetUser.ID
		}
		authn.NotifyLogin(ctx, event)
	}()

	// Find user by username
	users := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&users); err != nil {
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "invalid username or password", err)
	}
	if len(users) == 0 {
		return nil, service.NewError(http.StatusUnauthorized, "invalid username or password")
	}
	targetUser = users[0]

	// Check if user is enabled
	if targetUser.Status == modeliamuser.UserStatusInactive {
		return nil, service.NewError(http.StatusForbidden, "account disabled")
	}
	if targetUser.Status == modeliamuser.UserStatusLocked {
		return nil, service.NewError(http.StatusForbidden, "account locked")
	}

	credential, err := LoadPasswordCredential(ctx, targetUser.ID)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "invalid username or password", err)
	}
	if credential.LockedUntil != nil && credential.LockedUntil.After(time.Now()) {
		return nil, service.NewError(http.StatusForbidden, "account locked")
	}

	// Verify password
	if err = VerifyPasswordCredential(ctx, credential, req.Password); err != nil {
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "invalid username or password", err)
	}
	// Resolved for every login, not only the ones that name a tenant: the
	// principal in the response reports it, and it is what exempts a system
	// root from the tenant membership check below.
	systemRoot, err := rbac.RBAC().HasSystemRole(ctx, targetUser.ID, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID != "" && !systemRoot {
		if err = ensureLoginTenantMembership(ctx, targetUser.ID, tenantID); err != nil {
			return nil, err
		}
	}

	// The second-factor gate runs only after every first-factor check passed,
	// so a failed second factor never reveals more than a failed password. The
	// installed verifier owns the client-facing error shape.
	if svcErr := verifyLoginSecondFactor(ctx, targetUser.ID, authn.LoginSecondFactor{
		TOTPCode:   req.TOTPCode,
		BackupCode: req.BackupCode,
	}); svcErr != nil {
		return nil, svcErr
	}

	now := time.Now()
	// Create session
	sessionID, err := serviceiamsession.NewSessionID()
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to create session id", err)
	}
	expire := serviceiamsession.GetSessionExpiration()
	expiresAt := now.Add(expire)

	// Create session data for local user
	sessionData := modeliamsession.Session{
		ID:                 sessionID,
		UserID:             targetUser.ID,
		Username:           targetUser.Username,
		TenantID:           tenantID,
		MustChangePassword: credential.MustChangePassword,
		ClientIP:           ctx.ClientIP(),
		UserAgent:          ctx.UserAgent(),
		OS:                 ua.OS(),
		Platform:           ua.Platform(),
		EngineName:         engineName,
		BrowserName:        browserName,
		IssuedAt:           now,
		LastSeenAt:         now,
		ExpiresAt:          expiresAt,
	}
	if err = serviceiamsession.Store.SaveSession(ctx, sessionData, expire); err != nil {
		return nil, err
	}
	if err = serviceiamsession.Store.IndexSession(ctx, sessionData); err != nil {
		// A snapshot no index names can never be listed or revoked, so the
		// session is dropped rather than left behind unreachable.
		_, _ = serviceiamsession.Store.DeleteSession(ctx, sessionID)
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to track user session", err)
	}

	credential.FailedLoginCount = 0
	if err = database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithoutHook().
		WithSelect(colUserID, colFailedLoginCount).
		Update(credential); err != nil {
		log.Warnz("failed to update password credential statistics", zap.Error(err))
	}

	serviceiamsession.SetCookie(ctx, sessionID, expire)

	log.Infoz("user logged in successfully", zap.String("username", req.Username), zap.String("user_id", targetUser.ID))

	email := ""
	emailIdentity, err := LoadEmailIdentity(ctx, targetUser.ID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			log.Warnz("failed to load email identity for login response", zap.String("user_id", targetUser.ID), zap.Error(err))
		}
	} else {
		email = emailIdentity.Email
	}

	return serviceiamsession.BuildAuthenticatedSessionRsp(sessionData, targetUser, email, now, systemRoot), nil
}

// verifyLoginSecondFactor runs the authn second-factor gate and shapes its
// outcome as a service error: the installed verifier already answers with one
// per the authn contract and is passed through untouched, while anything else
// is an infrastructure failure reported as 500.
func verifyLoginSecondFactor(ctx *types.ServiceContext, userID string, factor authn.LoginSecondFactor) *service.Error {
	err := authn.VerifyLoginSecondFactor(ctx, userID, factor)
	if err == nil {
		return nil
	}
	var svcErr *service.Error
	if errors.As(err, &svcErr) {
		return svcErr
	}
	return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
}

// ensureLoginTenantMembership refuses a login that names a tenant the user holds
// no role in. The system root exemption is applied by the caller, which resolves
// that fact for the response anyway.
func ensureLoginTenantMembership(ctx *types.ServiceContext, userID string, tenantID string) error {
	roles, err := rbac.RBAC().RolesForSubject(ctx, tenantID, userID)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if len(roles) == 0 {
		return service.NewError(http.StatusForbidden, "user is not a member of tenant")
	}
	return nil
}
