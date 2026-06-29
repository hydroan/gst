package serviceiamsession

import (
	"net/http"
	"sort"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const sessionsDeleteOthersID = "others"

// SessionListService handles retrieval of all active sessions for the current authenticated user.
type SessionListService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionListReq, *modeliamsession.SessionListRsp]
}

// SessionGetService handles retrieval of a specified session for the current authenticated user.
type SessionGetService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionGetReq, *modeliamsession.SessionGetRsp]
}

// SessionDeleteService handles invalidation of a specified session for the current authenticated user.
type SessionDeleteService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionDeleteReq, *modeliamsession.SessionDeleteRsp]
}

// SessionDeleteAllService handles invalidation of all sessions for the current authenticated user.
type SessionDeleteAllService struct {
	service.Base[*modeliamsession.Session2, *modeliamsession.SessionDeleteAllReq, *modeliamsession.SessionDeleteAllRsp]
}

// List returns all active sessions for the current authenticated user.
func (s *SessionListService) List(ctx *types.ServiceContext, req *modeliamsession.SessionListReq) (rsp *modeliamsession.SessionListRsp, err error) {
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

	return &modeliamsession.SessionListRsp{
		Items: items,
		Total: int64(len(items)),
	}, nil
}

// Get returns the detail of a specified session for the current authenticated user.
func (s *SessionGetService) Get(ctx *types.ServiceContext, req *modeliamsession.SessionGetReq) (rsp *modeliamsession.SessionGetRsp, err error) {
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
