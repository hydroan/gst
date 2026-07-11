package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// SessionGetService handles retrieval of a specified session for the current authenticated user.
type SessionGetService struct {
	service.Base[*modeliamsession.Session2, *model.Empty, *modeliamsession.SessionGetRsp]
}

// Get returns the detail of a specified session for the current authenticated user.
func (s *SessionGetService) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamsession.SessionGetRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	currentSessionID, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, service.NewError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := redis.Cache[modeliamsession.Session]().WithContext(ctx).Get(modeliamsession.SessionIDKey(targetSessionID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to load target session", err)
		return nil, err
	}
	if err = SessionManager.Validate(targetSessionID, targetSession); err != nil {
		_, _ = SessionManager.Delete(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}
	if targetSession.UserID != currentSession.UserID {
		return nil, service.NewError(http.StatusForbidden, "forbidden")
	}

	return &modeliamsession.SessionGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}
