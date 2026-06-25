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

// CurrentListService handles retrieval of the current authenticated session.
type CurrentListService struct {
	service.Base[*model.Empty, *modeliamsession.CurrentListReq, *modeliamsession.CurrentListRsp]
}

// List returns the current authenticated session together with the latest user snapshot.
func (s *CurrentListService) List(ctx *types.ServiceContext, req *modeliamsession.CurrentListReq) (rsp *modeliamsession.CurrentListRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	sessionID, session, err := GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil || user.GetID() == "" {
		log.Error("failed to load user for current session")
		return nil, service.NewError(http.StatusUnauthorized, "session invalid")
	}
	switch user.Status {
	case modeliamuser.UserStatusInactive:
		return nil, service.NewError(http.StatusForbidden, "account disabled")
	case modeliamuser.UserStatusLocked:
		return nil, service.NewError(http.StatusForbidden, "account locked")
	}

	return buildCurrentListRsp(session, sessionID, &modeliamsession.CurrentPrincipal{
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
	log := s.WithServiceContext(ctx, ctx.GetPhase())

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

// buildCurrentListRsp builds the API response for getting the current session from the stored session snapshot.
func buildCurrentListRsp(session modeliamsession.Session, fallbackSessionID string, principal *modeliamsession.CurrentPrincipal) *modeliamsession.CurrentListRsp {
	if principal == nil {
		principal = &modeliamsession.CurrentPrincipal{}
	}

	return &modeliamsession.CurrentListRsp{
		Session:   buildCurrentSessionView(session, fallbackSessionID),
		Principal: *principal,
	}
}
