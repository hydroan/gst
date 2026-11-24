package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/modulemodel/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// SessionGetService handles retrieval of a specified session for the current authenticated user.
type SessionGetService struct {
	service.Base[*modeliamsession.SelfSession, *model.Empty, *modeliamsession.SessionGetRsp]
}

// Get returns the detail of a specified session for the current authenticated user.
func (s *SessionGetService) Get(ctx *gst.ServiceContext, req *model.Empty) (rsp *modeliamsession.SessionGetRsp, err error) {
	currentSessionID, currentSession, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, gst.NewError(http.StatusBadRequest, sessionIDRequiredMsg)
	}

	targetSession, err := Store.LoadSession(ctx, targetSessionID)
	if err != nil {
		if errors.Is(err, gst.ErrEntryNotFound) {
			return nil, gst.NewError(http.StatusNotFound, sessionNotFoundMsg)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLoadTargetSessionMsg, err)
	}
	if err = ValidateSession(targetSessionID, targetSession); err != nil {
		_, _ = Store.DeleteSession(ctx, targetSessionID)
		return nil, gst.NewError(http.StatusNotFound, sessionNotFoundMsg)
	}
	if targetSession.UserID != currentSession.UserID {
		return nil, gst.NewError(http.StatusForbidden, forbiddenMsg)
	}

	return &modeliamsession.SessionGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}
