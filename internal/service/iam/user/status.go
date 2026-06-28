package serviceiamuser

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type UserStatusPatchService struct {
	service.Base[*model.Empty, *modeliamuser.UserStatusPatchReq, *modeliamuser.UserStatusPatchRsp]
}

func (s *UserStatusPatchService) Patch(ctx *types.ServiceContext, req *modeliamuser.UserStatusPatchReq) (rsp *modeliamuser.UserStatusPatchRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("user status patch")

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}
	switch req.Status {
	case modeliamuser.UserStatusActive, modeliamuser.UserStatusInactive, modeliamuser.UserStatusLocked:
	default:
		return nil, errors.New("invalid status: must be active, inactive, or locked")
	}

	actor, target, err := LoadPrivilegedActorAndTarget(ctx, targetUserID)
	if err != nil {
		log.Error("failed to resolve actor or target user", err)
		return nil, err
	}

	if err = MayManageProtectedUser(actor, target); err != nil {
		log.Error("user status change denied", err)
		return nil, err
	}

	if target.Status == req.Status {
		// Still revoke sessions when the target state is inactive or locked so Redis cannot drift.
		if shouldInvalidateUserSessions(req.Status) {
			serviceiamsession.InvalidateUserSessions(ctx, targetUserID)
		} else {
			serviceiamsession.InvalidateUserStateCache(ctx, targetUserID)
		}
		return &modeliamuser.UserStatusPatchRsp{Msg: "user status unchanged"}, nil
	}

	target.Status = req.Status
	if err = database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect("username", "status").
		Update(target); err != nil {
		log.Error("failed to update user status", err)
		return nil, errors.Wrap(err, "failed to update user status")
	}

	if shouldInvalidateUserSessions(req.Status) {
		serviceiamsession.InvalidateUserSessions(ctx, targetUserID)
	} else {
		serviceiamsession.InvalidateUserStateCache(ctx, targetUserID)
	}

	log.Info("user status updated", "target_user_id", targetUserID, "status", req.Status, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamuser.UserStatusPatchRsp{Msg: "user status updated successfully"}, nil
}
