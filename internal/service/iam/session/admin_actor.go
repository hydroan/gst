package serviceiamsession

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// ensureAdminSessionActor verifies that the current session belongs to a privileged IAM actor.
func ensureAdminSessionActor(ctx *types.ServiceContext) error {
	_, session, err := GetCurrentSession(ctx)
	if err != nil {
		return err
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil {
		return service.NewError(http.StatusUnauthorized, "session invalid")
	}

	if session.UserID == consts.AUTHZ_USER_ROOT {
		return nil
	}
	if user.IsSuperuser != nil && *user.IsSuperuser {
		return nil
	}

	return service.NewError(http.StatusForbidden, "forbidden")
}
