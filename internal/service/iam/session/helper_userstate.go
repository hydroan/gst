package serviceiamsession

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

const sessionUserStateTTL = 5 * time.Second

type sessionUserState struct {
	Status             modeliamuser.UserStatus `json:"status"`
	MustChangePassword bool                    `json:"must_change_password"`
}

// ValidateSessionUserState refreshes the mutable user state required to keep using a session.
func ValidateSessionUserState(ctx context.Context, session modeliamsession.Session) (modeliamsession.Session, error) {
	if session.UserID == "" {
		return session, service.NewError(http.StatusUnauthorized, "session invalid")
	}

	state, ok := loadCachedSessionUserState(ctx, session.UserID)
	if !ok {
		var err error
		state, ok, err = refreshSessionUserState(ctx, session.UserID)
		if err != nil {
			return session, err
		}
		if !ok {
			return session, nil
		}
	}

	session.MustChangePassword = state.MustChangePassword
	return session, ensureSessionUserActive(&modeliamuser.User{Status: state.Status})
}

// InvalidateUserStateCache removes the short-lived mutable user-state cache for a user.
func InvalidateUserStateCache(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	_ = redis.Cache[sessionUserState]().
		WithContext(redisContext(ctx)).
		Delete(modeliamsession.SessionUserStateKey(userID))
}

func loadCachedSessionUserState(ctx context.Context, userID string) (sessionUserState, bool) {
	state, err := redis.Cache[sessionUserState]().
		WithContext(redisContext(ctx)).
		Get(modeliamsession.SessionUserStateKey(userID))
	if err == nil {
		return state, true
	}
	if !errors.Is(err, types.ErrEntryNotFound) {
		zap.S().Warnw("failed to load iam session user state cache", "user_id", userID, "error", err)
	}
	return sessionUserState{}, false
}

func refreshSessionUserState(ctx context.Context, userID string) (sessionUserState, bool, error) {
	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(user, userID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return sessionUserState{}, false, service.NewError(http.StatusUnauthorized, "session invalid")
		}
		zap.S().Warnw("failed to refresh iam session user state", "user_id", userID, "error", err)
		return sessionUserState{}, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to refresh session user state", err)
	}

	state := sessionUserState{
		Status:             user.Status,
		MustChangePassword: user.MustChangePassword,
	}
	if err := redis.Cache[sessionUserState]().
		WithContext(redisContext(ctx)).
		Set(modeliamsession.SessionUserStateKey(userID), state, sessionUserStateTTL); err != nil {
		zap.S().Warnw("failed to cache iam session user state", "user_id", userID, "error", err)
	}
	return state, true, nil
}
