package serviceiamuser

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// Column references for the narrow status write below; module sources carry
// no generated Cols vars, so the references are declared here.
var (
	colUsername   = types.NewColumn[string]("username")
	colUserStatus = types.NewColumn[modeliamuser.UserStatus]("status")
)

type UserStatusPatchService struct {
	service.Base[*modeliamuser.User, *modeliamuser.UserStatusPatchReq, *modeliamuser.UserStatusPatchRsp]
}

func (u *UserStatusPatchService) Patch(ctx *types.ServiceContext, req *modeliamuser.UserStatusPatchReq) (rsp *modeliamuser.UserStatusPatchRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user status patch")

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}
	switch req.Status {
	case modeliamuser.UserStatusActive, modeliamuser.UserStatusInactive, modeliamuser.UserStatusLocked:
	default:
		return nil, service.NewError(http.StatusBadRequest, "invalid status: must be active, inactive, or locked")
	}

	actor, target, err := LoadActorAndTarget(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	if err = adminauth.EnsureTenantAdmin(ctx, actor, target); err != nil {
		return nil, err
	}

	if target.Status == req.Status {
		// Still revoke sessions when the target state is inactive or locked so Redis cannot drift.
		revokeSessionsForStatus(ctx, log, req.Status, targetUserID)
		return &modeliamuser.UserStatusPatchRsp{Msg: "user status unchanged"}, nil
	}

	target.Status = req.Status
	if err = database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect(colUsername, colUserStatus).
		Update(target); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update user status", err)
	}

	revokeSessionsForStatus(ctx, log, req.Status, targetUserID)

	log.Info("user status updated", "target_user_id", targetUserID, "status", req.Status, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamuser.UserStatusPatchRsp{Msg: "user status updated successfully"}, nil
}

// revokeSessionsForStatus applies a status change to the target's live sessions.
//
// The row is already written by the time this runs, so a storage failure is
// logged rather than returned: failing the request would report a change that
// did happen as one that did not, and the user-state cache expiring on its own
// is the backstop either way.
func revokeSessionsForStatus(ctx *types.ServiceContext, log types.Logger, status modeliamuser.UserStatus, targetUserID string) {
	if !shouldInvalidateUserSessions(status) {
		serviceiamsession.Store.DropUserState(ctx, targetUserID)
		return
	}
	if err := serviceiamsession.Store.DeleteUserSessions(ctx, targetUserID); err != nil {
		log.Warn("failed to revoke sessions after user status change", err)
	}
}
