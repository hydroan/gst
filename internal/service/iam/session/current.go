package serviceiamsession

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

// CurrentGetService handles retrieval of the current authenticated session.
type CurrentGetService struct {
	service.Base[*model.Empty, *modeliamsession.CurrentGetReq, *modeliamsession.CurrentGetRsp]
}

// Get returns the current authenticated session together with the latest user snapshot.
func (s *CurrentGetService) Get(ctx *types.ServiceContext, req *modeliamsession.CurrentGetReq) (rsp *modeliamsession.CurrentGetRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	sessionID, session, err := GetCurrentSession(ctx)
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

	return buildCurrentGetRsp(session, sessionID, &modeliamsession.CurrentPrincipal{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              util.Deref(user.Email),
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		Status:             string(user.Status),
		MustChangePassword: user.MustChangePassword,
	}), nil
}

// CurrentDeleteService handles invalidation of the current authenticated session.
type CurrentDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.CurrentDeleteReq, *modeliamsession.CurrentDeleteRsp]
}

// Delete invalidates the current authenticated session and clears the session cookie.
func (s *CurrentDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.CurrentDeleteReq) (rsp *modeliamsession.CurrentDeleteRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	sessionID, err := ReadSessionID(ctx)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if _, err = DeleteSession(ctx, sessionID); err != nil {
		log.Error("failed to delete current session", err)
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}

	ClearSessionCookie(ctx)

	return &modeliamsession.CurrentDeleteRsp{}, nil
}

// buildCurrentGetRsp builds the API response for getting the current session from the stored session snapshot.
func buildCurrentGetRsp(session modeliamsession.Session, fallbackSessionID string, principal *modeliamsession.CurrentPrincipal) *modeliamsession.CurrentGetRsp {
	if principal == nil {
		principal = &modeliamsession.CurrentPrincipal{}
	}

	return &modeliamsession.CurrentGetRsp{
		Session:   buildSessionView(session, fallbackSessionID),
		Principal: *principal,
	}
}
