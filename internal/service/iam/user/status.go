package serviceiamuser

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
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
		if shouldInvalidateUserSessions(req.Status) {
			modeliamsession.InvalidateUserSessions(ctx, targetUserID)
		} else {
			modeliamsession.InvalidateUserStateCache(ctx, targetUserID)
		}
		return &modeliamuser.UserStatusPatchRsp{Msg: "user status unchanged"}, nil
	}

	target.Status = req.Status
	if err = database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect("username", "status").
		Update(target); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update user status", err)
	}

	if shouldInvalidateUserSessions(req.Status) {
		modeliamsession.InvalidateUserSessions(ctx, targetUserID)
	} else {
		modeliamsession.InvalidateUserStateCache(ctx, targetUserID)
	}

	log.Info("user status updated", "target_user_id", targetUserID, "status", req.Status, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamuser.UserStatusPatchRsp{Msg: "user status updated successfully"}, nil
}
