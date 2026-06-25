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
	"github.com/hydroan/gst/types/consts"
)

// UserService handles CRUD operations for IAM users.
type UserService struct {
	service.Base[*modeliamuser.User, *modeliamuser.User, *modeliamuser.User]
}

// UserPatchService handles allow-listed profile patches for IAM users.
type UserPatchService struct {
	service.Base[*model.Empty, *modeliamuser.UserPatchReq, *modeliamuser.User]
}

func (UserService) CreateBefore(ctx *types.ServiceContext, req *modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return err
	}
	return ensureUserCreateAllowed(actorID, req)
}

func (UserService) ListBefore(ctx *types.ServiceContext, _ *[]*modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	return ensureUserModuleSuperuser(actorID, actor)
}

func (UserService) GetBefore(ctx *types.ServiceContext, req *modeliamuser.User) error {
	return ensureUserTargetAccessible(ctx, req)
}

func (UserService) DeleteBefore(ctx *types.ServiceContext, req *modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return err
	}
	return ensureExistingUserTargetAllowed(ctx, actorID, req)
}

func (UserService) DeleteManyBefore(ctx *types.ServiceContext, users ...*modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return err
	}
	for _, user := range users {
		if err = ensureExistingUserTargetAllowed(ctx, actorID, user); err != nil {
			return err
		}
	}
	return nil
}

// DeleteAfter revokes Redis sessions for the deleted user. The controller only guarantees
// M with ID set (route/query/body id); no other fields are required.
func (UserService) DeleteAfter(ctx *types.ServiceContext, u *modeliamuser.User) error {
	if u == nil {
		return nil
	}
	serviceiamsession.InvalidateUserSessions(ctx, u.GetID())
	return nil
}

func (UserService) CreateManyBefore(ctx *types.ServiceContext, users ...*modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return err
	}
	for _, user := range users {
		if err = ensureUserCreateAllowed(actorID, user); err != nil {
			return err
		}
	}
	return nil
}

// DeleteManyAfter revokes sessions for each deleted user. Items contain only IDs from the batch request.
func (UserService) DeleteManyAfter(ctx *types.ServiceContext, users ...*modeliamuser.User) error {
	for _, u := range users {
		if u == nil {
			continue
		}
		serviceiamsession.InvalidateUserSessions(ctx, u.GetID())
	}
	return nil
}

// Patch updates only allow-listed user profile fields.
func (UserPatchService) Patch(ctx *types.ServiceContext, req *modeliamuser.UserPatchReq) (*modeliamuser.User, error) {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, service.NewError(http.StatusBadRequest, "user patch request is required")
	}

	targetID := ctx.Param("id")
	if targetID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	target := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(target, targetID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}
	if target.IsSuperuser != nil && *target.IsSuperuser && !isRootUserID(actorID) {
		return nil, userSuperuserTargetForbidden()
	}

	columns := []string{"username"}
	if req.Email != nil {
		target.Email = req.Email
		columns = append(columns, "email")
	}
	if req.Phone != nil {
		target.Phone = req.Phone
		columns = append(columns, "phone")
	}
	if req.FirstName != nil {
		target.FirstName = req.FirstName
		columns = append(columns, "first_name")
	}
	if req.LastName != nil {
		target.LastName = req.LastName
		columns = append(columns, "last_name")
	}
	if req.DisplayName != nil {
		target.DisplayName = req.DisplayName
		columns = append(columns, "display_name")
	}
	if req.Avatar != nil {
		target.Avatar = req.Avatar
		columns = append(columns, "avatar")
	}
	if req.Bio != nil {
		target.Bio = req.Bio
		columns = append(columns, "bio")
	}
	if req.Birthday != nil {
		target.Birthday = req.Birthday
		columns = append(columns, "birthday")
	}
	if req.Gender != nil {
		target.Gender = req.Gender
		columns = append(columns, "gender")
	}
	if len(columns) == 1 {
		return nil, service.NewError(http.StatusBadRequest, "patch fields are required")
	}

	target.SetUpdatedBy(ctx.Username())
	if err = database.Database[*modeliamuser.User](ctx).WithSelect(columns...).Update(target); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to patch user", err)
	}
	return target, nil
}

func userResourceActor(ctx *types.ServiceContext) (string, *modeliamuser.User, error) {
	_, session, err := serviceiamsession.GetCurrentSession(ctx)
	if err != nil {
		return "", nil, err
	}

	actor := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(actor, session.UserID); err != nil {
		return "", nil, service.NewErrorWithCause(http.StatusUnauthorized, "current user not found", err)
	}
	return actor.GetID(), actor, nil
}

func ensureUserModuleSuperuser(actorID string, actor *modeliamuser.User) error {
	if isRootUserID(actorID) {
		return nil
	}
	if actor != nil && actor.IsSuperuser != nil && *actor.IsSuperuser {
		return nil
	}
	return service.NewError(http.StatusForbidden, "superuser required")
}

func ensureUserTargetAccessible(ctx *types.ServiceContext, req *modeliamuser.User) error {
	actorID, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorID, actor); err != nil {
		return err
	}
	return ensureExistingUserTargetAllowed(ctx, actorID, req)
}

func ensureUserCreateAllowed(actorID string, req *modeliamuser.User) error {
	if req != nil && req.IsSuperuser != nil && *req.IsSuperuser && !isRootUserID(actorID) {
		return userSuperuserTargetForbidden()
	}
	return nil
}

func ensureExistingUserTargetAllowed(ctx *types.ServiceContext, actorID string, req *modeliamuser.User) error {
	if req == nil || req.GetID() == "" {
		return service.NewError(http.StatusBadRequest, "user id is required")
	}
	target := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(target, req.GetID()); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return service.NewError(http.StatusNotFound, "user not found")
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}
	if target.IsSuperuser != nil && *target.IsSuperuser && !isRootUserID(actorID) {
		return userSuperuserTargetForbidden()
	}
	return nil
}

func userSuperuserTargetForbidden() error {
	return service.NewError(http.StatusForbidden, "superuser is protected")
}

func isRootUserID(userID string) bool {
	return userID == consts.AUTHZ_USER_ROOT
}
