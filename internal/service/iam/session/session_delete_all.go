package serviceiamsession

import (
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
)

// SessionDeleteAllService handles invalidation of all sessions for the current authenticated user.
type SessionDeleteAllService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionDeleteAllReq, *modeliamsession.SessionDeleteAllRsp]
}

// Delete invalidates all sessions for the current authenticated user.
func (s *SessionDeleteAllService) Delete(ctx *gst.ServiceContext, req *modeliamsession.SessionDeleteAllReq) (rsp *modeliamsession.SessionDeleteAllRsp, err error) {
	_, currentSession, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	if err = Store.DeleteUserSessions(ctx, currentSession.UserID); err != nil {
		return nil, err
	}

	ClearCookie(ctx)

	return &modeliamsession.SessionDeleteAllRsp{}, nil
}
