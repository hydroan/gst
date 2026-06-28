package serviceiamsession

import (
	"net/http"
	"sort"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const sessionsDeleteOthersID = "others"

// SessionsListService handles retrieval of all active sessions for the current authenticated user.
type SessionsListService struct {
	service.Base[*model.Empty, *modeliamsession.SessionsListReq, *modeliamsession.SessionsListRsp]
}

// SessionsGetService handles retrieval of a specified session for the current authenticated user.
type SessionsGetService struct {
	service.Base[*model.Empty, *modeliamsession.SessionsGetReq, *modeliamsession.SessionsGetRsp]
}

// SessionsDeleteService handles invalidation of a specified session for the current authenticated user.
type SessionsDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.SessionsDeleteReq, *modeliamsession.SessionsDeleteRsp]
}

// SessionsDeleteAllService handles invalidation of all sessions for the current authenticated user.
type SessionsDeleteAllService struct {
	service.Base[*model.Empty, *modeliamsession.SessionsDeleteAllReq, *modeliamsession.SessionsDeleteAllRsp]
}

// List returns all active sessions for the current authenticated user.
func (s *SessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.SessionsListReq) (rsp *modeliamsession.SessionsListRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	// SessionManager.Current already guarantees that the resolved session is bound to
	// an authenticated user, so the service can directly use currentSession.UserID.
	currentSessionID, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	sessionIDs, err := listUserSessionIDs(ctx, currentSession.UserID)
	if err != nil {
		log.Error("failed to list user sessions", err)
		return nil, err
	}

	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	items := make([]modeliamsession.SessionView, 0, len(sessionIDs))
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == currentSessionID {
			items = append(items, buildSessionView(currentSession, currentSessionID))
			continue
		}
		sessionKey := modeliamsession.SessionIDKey(sessionID)
		session, getErr := cache.Get(sessionKey)
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				removeStaleSessionIndexes(ctx, currentSession.UserID, sessionID)
				continue
			}
			log.Error("failed to load session from redis", getErr)
			return nil, getErr
		}
		if validateErr := SessionManager.Validate(sessionID, session); validateErr != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			continue
		}
		items = append(items, buildSessionView(session, currentSessionID))
	}

	sort.Slice(items, func(i, j int) bool {
		left := sessionViewActiveAt(items[i])
		right := sessionViewActiveAt(items[j])
		if left.Equal(right) {
			return items[i].ID > items[j].ID
		}
		return left.After(right)
	})

	return &modeliamsession.SessionsListRsp{
		Items: items,
		Total: int64(len(items)),
	}, nil
}

// Get returns the detail of a specified session for the current authenticated user.
func (s *SessionsGetService) Get(ctx *types.ServiceContext, req *modeliamsession.SessionsGetReq) (rsp *modeliamsession.SessionsGetRsp, err error) {
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

	return &modeliamsession.SessionsGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}

// Delete invalidates a specified session for the current authenticated user.
// DELETE /api/iam/sessions/:id revokes the target session, while
// DELETE /api/iam/sessions/others revokes every other indexed session of the
// same user and keeps the current cookie-backed session active. The endpoint
// remains idempotent: deleting a missing session still returns success.
func (s *SessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionsDeleteReq) (rsp *modeliamsession.SessionsDeleteRsp, err error) {
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
		if err = deleteUserSessionsExceptCurrent(ctx, currentSession.UserID, currentSessionID); err != nil {
			log.Error("failed to delete other sessions", err)
			return nil, err
		}
		return &modeliamsession.SessionsDeleteRsp{}, nil
	}

	targetSession, err := redis.Cache[modeliamsession.Session]().WithContext(ctx).Get(modeliamsession.SessionIDKey(targetSessionID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			if targetSessionID == currentSessionID {
				SessionManager.ClearCookie(ctx)
			}
			return &modeliamsession.SessionsDeleteRsp{}, nil
		}
		log.Error("failed to load target session", err)
		return nil, err
	}
	if err = SessionManager.Validate(targetSessionID, targetSession); err != nil {
		_, _ = SessionManager.Delete(ctx, targetSessionID)
		if targetSessionID == currentSessionID {
			SessionManager.ClearCookie(ctx)
		}
		return &modeliamsession.SessionsDeleteRsp{}, nil
	}
	if targetSession.UserID != currentSession.UserID {
		return nil, service.NewError(http.StatusForbidden, "forbidden")
	}

	if _, err = SessionManager.Delete(ctx, targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			if targetSessionID == currentSessionID {
				SessionManager.ClearCookie(ctx)
			}
			return &modeliamsession.SessionsDeleteRsp{}, nil
		}
		log.Error("failed to delete target session", err)
		return nil, err
	}
	if targetSessionID == currentSessionID {
		SessionManager.ClearCookie(ctx)
	}

	return &modeliamsession.SessionsDeleteRsp{}, nil
}

// Delete invalidates all sessions for the current authenticated user.
func (s *SessionsDeleteAllService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionsDeleteAllReq) (rsp *modeliamsession.SessionsDeleteAllRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	_, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	if err = deleteUserSessions(ctx, currentSession.UserID); err != nil {
		log.Error("failed to delete all sessions", err)
		return nil, err
	}

	SessionManager.ClearCookie(ctx)

	return &modeliamsession.SessionsDeleteAllRsp{}, nil
}
