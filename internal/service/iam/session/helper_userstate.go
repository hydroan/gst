package serviceiamsession

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
)

// ValidateSessionUserState refreshes the mutable user state required to keep using a session.
func ValidateSessionUserState(ctx context.Context, session modeliamsession.Session) (modeliamsession.Session, error) {
	if session.UserID == "" {
		return session, service.NewError(http.StatusUnauthorized, "session invalid")
	}

	state, ok := Store.LoadUserState(ctx, session.UserID)
	if !ok {
		var err error
		if state, err = refreshSessionUserState(ctx, session.UserID); err != nil {
			return session, err
		}
	}

	session.MustChangePassword = state.MustChangePassword
	return session, ensureSessionUserActive(&modeliamuser.User{Status: state.Status})
}

// refreshSessionUserState reads the mutable user state from the database and
// caches it.
//
// A user or credential row that is gone is reported as an invalid session
// rather than as an absent state: nothing else deletes a session when its owner
// is deleted, so this refusal is what ends the sessions of a deleted user.
func refreshSessionUserState(ctx context.Context, userID string) (UserState, error) {
	targetUser := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(targetUser, userID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return UserState{}, service.NewError(http.StatusUnauthorized, "session invalid")
		}
		zap.S().Warnw("failed to refresh iam session user state", "user_id", userID, "error", err)
		return UserState{}, service.NewErrorWithCause(http.StatusInternalServerError, "failed to refresh session user state", err)
	}

	credential, err := loadSessionPasswordCredential(ctx, userID)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return UserState{}, service.NewError(http.StatusUnauthorized, "session invalid")
		}
		zap.S().Warnw("failed to refresh iam session password credential state", "user_id", userID, "error", err)
		return UserState{}, service.NewErrorWithCause(http.StatusInternalServerError, "failed to refresh session user state", err)
	}

	state := UserState{
		Status:             targetUser.Status,
		MustChangePassword: credential.MustChangePassword,
	}
	Store.SaveUserState(ctx, userID, state)
	return state, nil
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
		return "", service.NewErrorWithCause(http.StatusInternalServerError, "failed to load email identity", err)
	}
	return identity.Email, nil
}
