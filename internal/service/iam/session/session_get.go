package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// SessionGetService handles retrieval of a specified session for the current authenticated user.
type SessionGetService struct {
	service.Base[*modeliamsession.Session2, *model.Empty, *modeliamsession.SessionGetRsp]
}

// Get returns the detail of a specified session for the current authenticated user.
func (s *SessionGetService) Get(ctx *gst.ServiceContext, req *model.Empty) (rsp *modeliamsession.SessionGetRsp, err error) {
	currentSessionID, currentSession, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, service.NewError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := Store.LoadSession(ctx, targetSessionID)
	if err != nil {
		if errors.Is(err, gst.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target session", err)
	}
	if err = ValidateSession(targetSessionID, targetSession); err != nil {
		_, _ = Store.DeleteSession(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}
	if targetSession.UserID != currentSession.UserID {
		return nil, service.NewError(http.StatusForbidden, "forbidden")
	}

	return &modeliamsession.SessionGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}
