package serviceiamsession

import (
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// SessionDeleteAllService handles invalidation of all sessions for the current authenticated user.
type SessionDeleteAllService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionDeleteAllReq, *modeliamsession.SessionDeleteAllRsp]
}

// Delete invalidates all sessions for the current authenticated user.
func (s *SessionDeleteAllService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionDeleteAllReq) (rsp *modeliamsession.SessionDeleteAllRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	_, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	if err = DeleteUserSessions(ctx, currentSession.UserID); err != nil {
		log.Error("failed to delete all sessions", err)
		return nil, err
	}

	SessionManager.ClearCookie(ctx)

	return &modeliamsession.SessionDeleteAllRsp{}, nil
}
