package serviceiamaccount

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// privilegedActor reports whether the actor can manage privileged account operations.
func privilegedActor(actor *modeliamuser.User, username string) bool {
	if username == consts.AUTHZ_USER_ROOT || username == consts.AUTHZ_USER_ADMIN {
		return true
	}
	return actor.IsSuperuser != nil && *actor.IsSuperuser
}

// mayManageProtectedUser allows privileged actors to act on another user; superuser targets require root or admin.
func mayManageProtectedUser(actorUsername string, actor, target *modeliamuser.User) error {
	if !privilegedActor(actor, actorUsername) {
		return errors.New("forbidden: superuser privileges required")
	}
	if target.IsSuperuser != nil && *target.IsSuperuser {
		if actorUsername != consts.AUTHZ_USER_ROOT && actorUsername != consts.AUTHZ_USER_ADMIN {
			return errors.New("forbidden: only root or admin may modify a superuser")
		}
	}
	return nil
}

// loadPrivilegedActorAndTarget resolves the current actor from session context and loads the requested target user.
func loadPrivilegedActorAndTarget(ctx *types.ServiceContext, targetUserID string) (string, *modeliamuser.User, *modeliamuser.User, error) {
	_, session, err := serviceiamsession.GetCurrentSession(ctx)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "invalid session")
	}

	actorUsername := session.Username
	if actorUsername == "" {
		actorUsername = ctx.Username
	}
	if actorUsername == "" {
		return "", nil, nil, errors.New("actor username not found")
	}

	actors := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).
		WithLimit(1).
		WithQuery(&modeliamuser.User{Username: actorUsername}).
		List(&actors); err != nil {
		return "", nil, nil, errors.Wrap(err, "database error")
	}
	if len(actors) == 0 {
		return "", nil, nil, errors.New("actor user not found")
	}

	target := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(target, targetUserID); err != nil {
		return "", nil, nil, errors.Wrap(err, "user not found")
	}

	return actorUsername, actors[0], target, nil
}

// shouldInvalidateUserSessions returns whether a user status transition must revoke all active sessions.
func shouldInvalidateUserSessions(status modeliamuser.UserStatus) bool {
	return status == modeliamuser.UserStatusInactive || status == modeliamuser.UserStatusLocked
}
