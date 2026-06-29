package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// SessionDeleteService handles invalidation of a specified session for the current authenticated user.
type SessionDeleteService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionDeleteReq, *modeliamsession.SessionDeleteRsp]
}

// Delete invalidates a specified session for the current authenticated user.
// DELETE /api/iam/sessions/:id revokes the target session, while
// DELETE /api/iam/sessions/others revokes every other indexed session of the
// same user and keeps the current cookie-backed session active. The endpoint
// remains idempotent: deleting a missing session still returns success.
func (s *SessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionDeleteReq) (rsp *modeliamsession.SessionDeleteRsp, err error) {
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
	if targetSessionID == sessionsDeleteOthersID {
		// DELETE /api/iam/sessions/others is a bulk self-service logout for
		// secondary sessions. The current cookie-backed session must survive so
		// the caller can continue using the API after the request completes.
		if err = DeleteUserSessionsExceptCurrent(ctx, currentSession.UserID, currentSessionID); err != nil {
			log.Error("failed to delete other sessions", err)
			return nil, err
		}
		return &modeliamsession.SessionDeleteRsp{}, nil
	}

	targetSession, err := redis.Cache[modeliamsession.Session]().WithContext(ctx).Get(modeliamsession.SessionIDKey(targetSessionID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			if targetSessionID == currentSessionID {
				SessionManager.ClearCookie(ctx)
			}
			return &modeliamsession.SessionDeleteRsp{}, nil
		}
		log.Error("failed to load target session", err)
		return nil, err
	}
	if err = SessionManager.Validate(targetSessionID, targetSession); err != nil {
		_, _ = SessionManager.Delete(ctx, targetSessionID)
		if targetSessionID == currentSessionID {
			SessionManager.ClearCookie(ctx)
		}
		return &modeliamsession.SessionDeleteRsp{}, nil
	}
	if targetSession.UserID != currentSession.UserID {
		return nil, service.NewError(http.StatusForbidden, "forbidden")
	}

	if _, err = SessionManager.Delete(ctx, targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			if targetSessionID == currentSessionID {
				SessionManager.ClearCookie(ctx)
			}
			return &modeliamsession.SessionDeleteRsp{}, nil
		}
		log.Error("failed to delete target session", err)
		return nil, err
	}
	if targetSessionID == currentSessionID {
		SessionManager.ClearCookie(ctx)
	}

	return &modeliamsession.SessionDeleteRsp{}, nil
}
