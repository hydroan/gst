package serviceiamsession

import (
	"net/http"
	"sort"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// SessionListService handles retrieval of all active sessions for the current authenticated user.
type SessionListService struct {
	service.Base[*modeliamsession.Session2, *model.Empty, *modeliamsession.SessionListRsp]
}

// List returns all active sessions for the current authenticated user.
func (s *SessionListService) List(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamsession.SessionListRsp, err error) {
	// SessionManager.Current already guarantees that the resolved session is bound to
	// an authenticated user, so the service can directly use currentSession.UserID.
	currentSessionID, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		return nil, err
	}

	sessionIDs, err := listUserSessionIDs(ctx, currentSession.UserID)
	if err != nil {
		return nil, err
	}

	cache := redis.Cache[modeliamsession.Session]()
	items := make([]modeliamsession.SessionView, 0, len(sessionIDs))
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == currentSessionID {
			items = append(items, buildSessionView(currentSession, currentSessionID))
			continue
		}
		sessionKey := modeliamsession.SessionIDKey(sessionID)
		sessionData, getErr := cache.Get(ctx, sessionKey)
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				_ = modeliamsession.RemoveSessionIndexes(ctx, currentSession.UserID, sessionID)
				continue
			}
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load session", getErr)
		}
		if validateErr := SessionManager.Validate(sessionID, sessionData); validateErr != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			continue
		}
		items = append(items, buildSessionView(sessionData, currentSessionID))
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
		Total: len(items),
	}, nil
}
