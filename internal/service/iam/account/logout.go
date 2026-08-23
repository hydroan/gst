package serviceiamaccount

import (
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authn"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/mssola/useragent"
)

// LogoutService handles logout requests for the current authenticated session.
type LogoutService struct {
	service.Base[*modeliamaccount.Logout, *model.Empty, *modeliamaccount.LogoutRsp]
}

// Create logs out the current session and always clears the session cookie on success.
func (l *LogoutService) Create(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamaccount.LogoutRsp, err error) {
	log := l.WithContext(ctx, ctx.Phase())

	sessionID, err := serviceiamsession.CookieSessionID(ctx)
	if err != nil {
		log.Error("failed to get session_id from cookie", err)
		serviceiamsession.ClearCookie(ctx)
		return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil // Return success even if no session
	}

	deletedSession, err := serviceiamsession.Store.DeleteSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			serviceiamsession.ClearCookie(ctx)
			return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil
		}

		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to logout", err)
	}

	// Only a logout that actually ended a session is a lifecycle event; the
	// cookie-less and already-expired branches above end nothing.
	ua := useragent.New(ctx.UserAgent())
	engineName, engineVersion := ua.Engine()
	browserName, browserVersion := ua.Browser()
	authn.NotifyLogin(ctx, authn.LoginEvent{
		Kind:           authn.LoginEventLoggedOut,
		UserID:         deletedSession.UserID,
		Username:       deletedSession.Username,
		TenantID:       deletedSession.TenantID,
		ClientIP:       ctx.ClientIP(),
		UserAgent:      ctx.UserAgent(),
		OS:             ua.OS(),
		Platform:       ua.Platform(),
		EngineName:     engineName,
		EngineVersion:  engineVersion,
		BrowserName:    browserName,
		BrowserVersion: browserVersion,
		At:             time.Now().UTC(),
	})

	serviceiamsession.ClearCookie(ctx)

	log.Info("user logged out successfully", "session_id", sessionID)
	return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil
}
