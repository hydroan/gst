package serviceiamuser

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// Column references for the narrow writes below; module sources carry no
// generated Cols vars, so the references are declared here.
var (
	colUsername   = types.NewColumn[string]("username")
	colUserStatus = types.NewColumn[modeliamuser.UserStatus]("status")
)

// AdminUserPatchService handles PATCH /iam/admin/users/:id for privileged
// administrators.
//
// It is the one route for updating a user, rather than a route per field. A
// per-field route reads as finer-grained authorization but is not: authorizing
// two paths where the framework authorizes one only splits the check, and the
// fields keep arriving through whichever of them a caller picks.
type AdminUserPatchService struct {
	service.Base[*modeliamuser.User, *modeliamuser.AdminUserPatchReq, *modeliamuser.AdminUserPatchRsp]
}

// Patch writes the fields the request named and leaves the rest alone.
func (u *AdminUserPatchService) Patch(ctx *types.ServiceContext, req *modeliamuser.AdminUserPatchReq) (rsp *modeliamuser.AdminUserPatchRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}
	if req.Username == nil && req.Status == nil {
		// Refused rather than accepted as a no-op: a request naming no field
		// looks the same as one whose field names are misspelled.
		return nil, service.NewError(http.StatusBadRequest, "no updatable field provided")
	}
	if req.Status != nil {
		switch *req.Status {
		case modeliamuser.UserStatusActive, modeliamuser.UserStatusInactive, modeliamuser.UserStatusLocked:
		default:
			return nil, service.NewError(http.StatusBadRequest, "invalid status: must be active, inactive, or locked")
		}
	}

	actor, target, err := serviceiamaccount.LoadActorAndTarget(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if err = adminauth.EnsureTenantAdmin(ctx, actor, target); err != nil {
		return nil, err
	}

	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			return nil, service.NewError(http.StatusBadRequest, "username cannot be empty")
		}
		target.Username = username
	}
	// Read before the write, because the write is what makes them equal and the
	// revocation below has to know whether the account just became unusable.
	statusChanged := req.Status != nil && target.Status != *req.Status
	if req.Status != nil {
		target.Status = *req.Status
	}

	if err = database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect(colUsername, colUserStatus).
		Update(target); err != nil {
		if errors.Is(err, database.ErrDuplicatedKey) {
			return nil, service.NewErrorWithCause(http.StatusConflict, "username already exists", err)
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update user", err)
	}

	// Run even when the status did not change: a request restating a status the
	// account already had is how an administrator retries a revocation that
	// Redis dropped the first time.
	if req.Status != nil {
		revokeSessionsForStatus(ctx, log, *req.Status, targetUserID)
	} else if req.Username != nil {
		// The username is part of the cached session snapshot, so the cache has
		// to be dropped or authentication keeps reporting the old one.
		serviceiamsession.Store.DropUserState(ctx, targetUserID)
	}

	view, err := buildAdminUserView(ctx, target)
	if err != nil {
		return nil, err
	}

	log.Info("user updated", "target_user_id", targetUserID, "status_changed", statusChanged, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamuser.AdminUserPatchRsp{User: view}, nil
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
