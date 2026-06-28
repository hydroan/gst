package serviceiamuser

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// EnsureRootActor allows only the built-in root user to run privileged user operations.
func EnsureRootActor(actor *modeliamuser.User) error {
	if actor == nil || actor.GetID() != consts.AUTHZ_USER_ROOT {
		return service.NewError(http.StatusForbidden, "root required")
	}
	return nil
}

// LoadActorAndTarget resolves the current actor from session context and loads the requested target user.
func LoadActorAndTarget(ctx *types.ServiceContext, targetUserID string) (*modeliamuser.User, *modeliamuser.User, error) {
	_, session, err := serviceiamsession.SessionManager.Current(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid session")
	}
	if session.UserID == "" {
		return nil, nil, service.NewError(http.StatusUnauthorized, "current user not found")
	}

	actor := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(actor, session.UserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, nil, service.NewError(http.StatusUnauthorized, "current user not found")
		}
		return nil, nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load current user", err)
	}

	target := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(target, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, nil, service.NewError(http.StatusNotFound, "user not found")
		}
		return nil, nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}

	return actor, target, nil
}

// shouldInvalidateUserSessions returns whether a user status transition must revoke all active sessions.
func shouldInvalidateUserSessions(status modeliamuser.UserStatus) bool {
	return status == modeliamuser.UserStatusInactive || status == modeliamuser.UserStatusLocked
}
