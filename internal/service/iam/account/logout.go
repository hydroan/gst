package serviceiamaccount

import (
	"fmt"

	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
)

// LogoutService handles logout requests for the current authenticated session.
type LogoutService struct {
	service.Base[*model.Empty, *model.Empty, *modeliamaccount.LogoutRsp]
}

// Create logs out the current session and always clears the session cookie on success.
func (s *LogoutService) Create(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamaccount.LogoutRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	sessionID, err := serviceiamsession.ReadSessionID(ctx)
	if err != nil {
		log.Error("failed to get session_id from cookie", err)
		serviceiamsession.ClearSessionCookie(ctx)
		return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil // Return success even if no session
	}

	session, err := serviceiamsession.DeleteSession(ctx.Context(), sessionID)

	// Parse user agent for logging
	ua := useragent.New(ctx.UserAgent())
	engineName, engineVersion := ua.Engine()
	browserName, browserVersion := ua.Browser()

	// Record logout log
	var userID, username string
	if err == nil {
		userID = session.UserID
		username = session.Username
	}

	if logErr := database.Database[*modellogmgmt.LoginLog](ctx).Create(&modellogmgmt.LoginLog{
		UserID:   userID,
		Username: username,
		ClientIP: ctx.ClientIP(),
		Status:   modellogmgmt.LoginStatusLogout,
		Source:   ctx.Request().UserAgent(),
		Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
		Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
		Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
	}); logErr != nil {
		log.Warnz("failed to write logout log", zap.Error(logErr))
	}

	if err != nil {
		log.Warnz("failed to delete session from redis", zap.Error(err))
	}

	serviceiamsession.ClearSessionCookie(ctx)

	log.Info("user logged out successfully", "session_id", sessionID)
	return &modeliamaccount.LogoutRsp{Msg: "logout successful"}, nil
}
