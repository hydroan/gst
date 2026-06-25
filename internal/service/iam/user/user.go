package serviceiamuser

import (
	"net/http"

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
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
		return err
	}
	return ensureUserCreateAllowed(actorUsername, req)
}

func (UserService) ListBefore(ctx *types.ServiceContext, _ *[]*modeliamuser.User) error {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	return ensureUserModuleSuperuser(actorUsername, actor)
}

func (UserService) GetBefore(ctx *types.ServiceContext, req *modeliamuser.User) error {
	return ensureUserTargetAccessible(ctx, req)
}

func (UserService) DeleteBefore(ctx *types.ServiceContext, req *modeliamuser.User) error {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
		return err
	}
	return ensureExistingUserTargetAllowed(ctx, actorUsername, req)
}

func (UserService) DeleteManyBefore(ctx *types.ServiceContext, users ...*modeliamuser.User) error {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
		return err
	}
	for _, user := range users {
		if err = ensureExistingUserTargetAllowed(ctx, actorUsername, user); err != nil {
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
	serviceiamsession.InvalidateUserSessions(ctx.Context(), u.GetID())
	return nil
}

func (UserService) CreateManyBefore(ctx *types.ServiceContext, users ...*modeliamuser.User) error {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
		return err
	}
	for _, user := range users {
		if err = ensureUserCreateAllowed(actorUsername, user); err != nil {
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
		serviceiamsession.InvalidateUserSessions(ctx.Context(), u.GetID())
	}
	return nil
}

// Patch updates only allow-listed user profile fields.
func (UserPatchService) Patch(ctx *types.ServiceContext, req *modeliamuser.UserPatchReq) (*modeliamuser.User, error) {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
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
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}
	if target.ID == "" {
		return nil, service.NewError(http.StatusNotFound, "user not found")
	}
	if target.IsSuperuser != nil && *target.IsSuperuser && !isRootOrAdmin(actorUsername) {
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
	if actor.ID == "" {
		return "", nil, service.NewError(http.StatusUnauthorized, "current user not found")
	}
	return actor.Username, actor, nil
}

func ensureUserModuleSuperuser(actorUsername string, actor *modeliamuser.User) error {
	if isRootOrAdmin(actorUsername) {
		return nil
	}
	if actor != nil && actor.IsSuperuser != nil && *actor.IsSuperuser {
		return nil
	}
	return service.NewError(http.StatusForbidden, "superuser required")
}

func ensureUserTargetAccessible(ctx *types.ServiceContext, req *modeliamuser.User) error {
	actorUsername, actor, err := userResourceActor(ctx)
	if err != nil {
		return err
	}
	if err = ensureUserModuleSuperuser(actorUsername, actor); err != nil {
		return err
	}
	return ensureExistingUserTargetAllowed(ctx, actorUsername, req)
}

func ensureUserCreateAllowed(actorUsername string, req *modeliamuser.User) error {
	if req != nil && req.IsSuperuser != nil && *req.IsSuperuser && !isRootOrAdmin(actorUsername) {
		return userSuperuserTargetForbidden()
	}
	return nil
}

func ensureExistingUserTargetAllowed(ctx *types.ServiceContext, actorUsername string, req *modeliamuser.User) error {
	if req == nil || req.GetID() == "" {
		return service.NewError(http.StatusBadRequest, "user id is required")
	}
	target := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(target, req.GetID()); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}
	if target.ID == "" {
		return service.NewError(http.StatusNotFound, "user not found")
	}
	if target.IsSuperuser != nil && *target.IsSuperuser && !isRootOrAdmin(actorUsername) {
		return userSuperuserTargetForbidden()
	}
	return nil
}

func userSuperuserTargetForbidden() error {
	return service.NewError(http.StatusForbidden, "superuser is protected")
}

func isRootOrAdmin(username string) bool {
	return username == consts.AUTHZ_USER_ROOT || username == consts.AUTHZ_USER_ADMIN
}
