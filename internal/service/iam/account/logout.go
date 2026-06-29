package serviceiamaccount

import (
	// "fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	// "github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	// modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	// "github.com/mssola/useragent"
	// "go.uber.org/zap"
)

// LogoutService handles logout requests for the current authenticated session.
type LogoutService struct {
	service.Base[*modeliamaccount.Logout, *modeliamaccount.LogoutReq, *modeliamaccount.LogoutRsp]
}

// Create logs out the current session and always clears the session cookie on success.
func (l *LogoutService) Create(ctx *types.ServiceContext, req *modeliamaccount.LogoutReq) (rsp *modeliamaccount.LogoutRsp, err error) {
	log := l.WithContext(ctx, ctx.Phase())

	sessionID, err := serviceiamsession.SessionManager.SessionID(ctx)
	if err != nil {
		log.Error("failed to get session_id from cookie", err)
		serviceiamsession.SessionManager.ClearCookie(ctx)
		return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil // Return success even if no session
	}

	deletedSession, err := serviceiamsession.SessionManager.Delete(ctx, sessionID)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			serviceiamsession.SessionManager.ClearCookie(ctx)
			return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil
		}

		log.Error("failed to delete session from redis", err)
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to logout", err)
	}

	// Logmgmt integration is disabled while IAM is decoupled from optional modules.
	//
	// ua := useragent.New(ctx.UserAgent())
	// engineName, engineVersion := ua.Engine()
	// browserName, browserVersion := ua.Browser()
	//
	// if logErr := database.Database[*modellogmgmt.LoginLog](ctx).Create(&modellogmgmt.LoginLog{
	// 	UserID:   session.UserID,
	// 	Username: session.Username,
	// 	ClientIP: ctx.ClientIP(),
	// 	Status:   modellogmgmt.LoginStatusLogout,
	// 	Source:   ctx.UserAgent(),
	// 	Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
	// 	Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
	// 	Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
	// }); logErr != nil {
	// 	log.Warnz("failed to write logout log", zap.Error(logErr))
	// }
	_ = deletedSession

	serviceiamsession.SessionManager.ClearCookie(ctx)

	log.Info("user logged out successfully", "session_id", sessionID)
	return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil
}
