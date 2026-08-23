package serviceiamaccount

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// LoadActor resolves the authenticated user for the current request.
//
// IAM admin APIs use the session snapshot only as the authentication source;
// they still reload the user row from the database so status, IDs, and system
// role checks operate on the current persisted user.
func LoadActor(ctx *types.ServiceContext) (*modeliamuser.User, error) {
	_, session, err := serviceiamsession.CurrentSession(ctx)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "invalid session", err)
	}
	if session.UserID == "" {
		return nil, service.NewError(http.StatusUnauthorized, "current user not found")
	}

	actor := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(actor, session.UserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusUnauthorized, "current user not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load current user", err)
	}
	return actor, nil
}

// LoadActorAndTarget resolves the actor and target users used by admin actions.
//
// It intentionally does not authorize the relationship between the two users.
// Callers must pass both users to adminauth.EnsureTenantAdmin so target-specific
// rules, such as tenant membership and system-root protection, are evaluated in
// one place.
func LoadActorAndTarget(ctx *types.ServiceContext, targetUserID string) (*modeliamuser.User, *modeliamuser.User, error) {
	actor, err := LoadActor(ctx)
	if err != nil {
		return nil, nil, err
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
