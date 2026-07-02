package serviceiamsession

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

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
	targetUser := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(targetUser, userID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return sessionUserState{}, false, service.NewError(http.StatusUnauthorized, "session invalid")
		}
		zap.S().Warnw("failed to refresh iam session user state", "user_id", userID, "error", err)
		return sessionUserState{}, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to refresh session user state", err)
	}

	credential, err := loadSessionPasswordCredential(ctx, userID)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return sessionUserState{}, false, service.NewError(http.StatusUnauthorized, "session invalid")
		}
		zap.S().Warnw("failed to refresh iam session password credential state", "user_id", userID, "error", err)
		return sessionUserState{}, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to refresh session user state", err)
	}

	state := sessionUserState{
		Status:             targetUser.Status,
		MustChangePassword: credential.MustChangePassword,
	}
	if err := redis.Cache[sessionUserState]().
		WithContext(redisContext(ctx)).
		Set(modeliamsession.SessionUserStateKey(userID), state, GetSessionUserStateTTL()); err != nil {
		zap.S().Warnw("failed to cache iam session user state", "user_id", userID, "error", err)
	}
	return state, true, nil
}

func loadSessionPasswordCredential(ctx context.Context, userID string) (*modeliamaccount.PasswordCredential, error) {
	credentials := make([]*modeliamaccount.PasswordCredential, 0, 1)
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.PasswordCredential{UserID: userID}).
		List(&credentials); err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return credentials[0], nil
}

func loadSessionEmailIdentity(ctx context.Context, userID string) (*modeliamaccount.EmailIdentity, error) {
	identities := make([]*modeliamaccount.EmailIdentity, 0, 1)
	if err := database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.EmailIdentity{UserID: userID}).
		List(&identities); err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return identities[0], nil
}

func loadSessionEmail(ctx context.Context, userID string) (string, error) {
	identity, err := loadSessionEmailIdentity(ctx, userID)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return identity.Email, nil
}
