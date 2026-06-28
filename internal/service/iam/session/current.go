package serviceiamsession

import (
	"net/http"
	"time"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// CurrentGetService handles retrieval of the current authenticated session.
type CurrentGetService struct {
	service.Base[*model.Empty, *modeliamsession.CurrentGetReq, *modeliamsession.CurrentGetRsp]
}

// Get returns the current authenticated session together with the latest user snapshot.
func (s *CurrentGetService) Get(ctx *types.ServiceContext, req *modeliamsession.CurrentGetReq) (rsp *modeliamsession.CurrentGetRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	_, session, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil {
		log.Error("failed to load user for current session")
		return nil, service.NewError(http.StatusUnauthorized, "session invalid")
	}
	if err = ensureSessionUserActive(user); err != nil {
		return nil, err
	}

	return BuildAuthenticatedSessionRsp(session, user, time.Now()), nil
}

// CurrentDeleteService handles invalidation of the current authenticated session.
type CurrentDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.CurrentDeleteReq, *modeliamsession.CurrentDeleteRsp]
}

// Delete invalidates the current authenticated session and clears the session cookie.
func (s *CurrentDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.CurrentDeleteReq) (rsp *modeliamsession.CurrentDeleteRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	sessionID, err := SessionManager.SessionID(ctx)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if _, err = SessionManager.Delete(ctx, sessionID); err != nil {
		log.Error("failed to delete current session", err)
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}

	SessionManager.ClearCookie(ctx)

	return &modeliamsession.CurrentDeleteRsp{}, nil
}
