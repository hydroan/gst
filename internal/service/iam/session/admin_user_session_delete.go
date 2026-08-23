package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminUserSessionDeleteService handles invalidation of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionDeleteService struct {
	service.Base[*modeliamsession.AdminUserSession, *modeliamsession.AdminUserSessionDeleteReq, *modeliamsession.AdminUserSessionDeleteRsp]
}

// Delete invalidates all indexed sessions of a specified user for a privileged administrator.
func (a *AdminUserSessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminUserSessionDeleteReq) (rsp *modeliamsession.AdminUserSessionDeleteRsp, err error) {
	_, currentSession, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	targetUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(targetUser, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", err)
	}
	if err = ensureAdminSessionTarget(ctx, targetUser); err != nil {
		return nil, err
	}

	if err = Store.DeleteUserSessions(ctx, targetUserID); err != nil {
		return nil, err
	}
	if currentSession.UserID == targetUserID {
		ClearCookie(ctx)
	}

	return &modeliamsession.AdminUserSessionDeleteRsp{}, nil
}
