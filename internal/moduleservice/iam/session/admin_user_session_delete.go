package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/modulemodel/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/modulemodel/iam/user"
	"github.com/hydroan/gst/service"
)

// AdminUserSessionDeleteService handles invalidation of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionDeleteService struct {
	service.Base[*modeliamsession.AdminUserSession, *modeliamsession.AdminUserSessionDeleteReq, *modeliamsession.AdminUserSessionDeleteRsp]
}

// Delete invalidates all indexed sessions of a specified user for a privileged administrator.
func (a *AdminUserSessionDeleteService) Delete(ctx *gst.ServiceContext, req *modeliamsession.AdminUserSessionDeleteReq) (rsp *modeliamsession.AdminUserSessionDeleteRsp, err error) {
	_, currentSession, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, gst.NewError(http.StatusBadRequest, userIDRequiredMsg)
	}

	targetUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(targetUser, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, gst.NewError(http.StatusNotFound, userNotFoundMsg)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLoadUserMsg, err)
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
