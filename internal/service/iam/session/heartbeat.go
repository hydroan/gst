package serviceiamsession

import (
	"fmt"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/mssola/useragent"
)

// HeartbeatService records client liveness for the current authenticated session.
type HeartbeatService struct {
	service.Base[*modeliamsession.Heartbeat, *modeliamsession.Heartbeat, *modeliamsession.Heartbeat]
}

// Create validates the current session and updates the online-user record without
// extending the Redis session lifetime.
func (s *HeartbeatService) Create(ctx *types.ServiceContext, req *modeliamsession.Heartbeat) (rsp *modeliamsession.Heartbeat, err error) {
	log := s.WithContext(ctx, ctx.GetPhase())

	if _, _, err = GetCurrentSession(ctx); err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	ua := useragent.New(ctx.UserAgent())
	engineName, engineVersion := ua.Engine()
	browserName, browserVersion := ua.Browser()

	if err = database.Database[*modeliamsession.OnlineUser](ctx).Update(&modeliamsession.OnlineUser{
		UserID:   ctx.UserID(),
		ClientIP: ctx.ClientIP(),
		Username: ctx.Username(),
		Source:   ctx.UserAgent(),
		Platform: fmt.Sprintf("%s %s", ua.Platform(), ua.OS()),
		Engine:   fmt.Sprintf("%s %s", engineName, engineVersion),
		Browser:  fmt.Sprintf("%s %s", browserName, browserVersion),
	}); err != nil {
		log.Error(err)
		return rsp, err
	}
	return &modeliamsession.Heartbeat{}, nil
}
