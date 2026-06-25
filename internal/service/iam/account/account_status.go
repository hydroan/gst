package serviceiamaccount

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type AccountStatusService struct {
	service.Base[*model.Empty, *modeliamaccount.AccountStatusReq, *modeliamaccount.AccountStatusRsp]
}

func (s *AccountStatusService) Create(ctx *types.ServiceContext, req *modeliamaccount.AccountStatusReq) (rsp *modeliamaccount.AccountStatusRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("account status create")

	if req.UserID == "" {
		return nil, errors.New("user_id is required")
	}
	switch req.Status {
	case modeliamuser.UserStatusActive, modeliamuser.UserStatusInactive, modeliamuser.UserStatusLocked:
	default:
		return nil, errors.New("invalid status: must be active, inactive, or locked")
	}

	actor, target, err := loadPrivilegedActorAndTarget(ctx, req.UserID)
	if err != nil {
		log.Error("failed to resolve actor or target user", err)
		return nil, err
	}

	if err = mayManageProtectedUser(actor, target); err != nil {
		log.Error("account status change denied", err)
		return nil, err
	}

	if target.Status == req.Status {
		// Still revoke sessions when the target state is inactive or locked so Redis cannot drift.
		if shouldInvalidateUserSessions(req.Status) {
			serviceiamsession.InvalidateUserSessions(ctx, req.UserID)
		}
		return &modeliamaccount.AccountStatusRsp{Msg: "account status unchanged"}, nil
	}

	target.Status = req.Status
	if err = database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect("username", "status").
		Update(target); err != nil {
		log.Error("failed to update user status", err)
		return nil, errors.Wrap(err, "failed to update account status")
	}

	if shouldInvalidateUserSessions(req.Status) {
		serviceiamsession.InvalidateUserSessions(ctx, req.UserID)
	}

	log.Info("account status updated", "target_user_id", req.UserID, "status", req.Status, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamaccount.AccountStatusRsp{Msg: "account status updated successfully"}, nil
}
