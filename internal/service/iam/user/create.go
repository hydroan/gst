package serviceiamuser

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminUserCreateService handles POST /iam/admin/users for privileged
// administrators.
//
// It is how an account comes into being in a deployment that does not offer
// public signup, which is most of them: the alternative was leaving every
// project to write its own user creation against the IAM tables.
type AdminUserCreateService struct {
	service.Base[*modeliamuser.User, *modeliamuser.AdminUserCreateReq, *modeliamuser.AdminUserCreateRsp]
}

// Create creates a user together with the credential it signs in with.
//
// Passing nil as the target to EnsureTenantAdmin checks endpoint permission
// only: there is no target user yet, and the one about to exist belongs to no
// tenant until authz binds it to a role. That binding is deliberately not made
// here — which tenants an account belongs to is authz's to record, and an IAM
// that wrote role bindings would own half of a decision it cannot see the rest
// of.
func (u *AdminUserCreateService) Create(ctx *types.ServiceContext, req *modeliamuser.AdminUserCreateReq) (rsp *modeliamuser.AdminUserCreateRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return nil, service.NewError(http.StatusBadRequest, "username is required")
	}

	actor, err := serviceiamaccount.LoadActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = adminauth.EnsureTenantAdmin(ctx, actor, nil); err != nil {
		return nil, err
	}

	newUser := &modeliamuser.User{
		Username: username,
		Status:   modeliamuser.UserStatusActive,
	}
	// A password someone else chose is a password two people know, so the
	// account is created owing a change unless the caller says otherwise.
	mustChangePassword := true
	if req.MustChangePassword != nil {
		mustChangePassword = *req.MustChangePassword
	}

	// The user, the credential and the email identity are one account. A
	// transaction is what keeps a half-created one — a user nobody can sign in
	// as, or a credential belonging to nobody — from being left behind.
	if err = database.Transaction(ctx, func(ctx context.Context) error {
		if createErr := database.Database[*modeliamuser.User](ctx).Create(newUser); createErr != nil {
			return createErr
		}

		credential, createErr := serviceiamaccount.NewPasswordCredential(ctx, newUser.ID, req.Password, mustChangePassword)
		if createErr != nil {
			return createErr
		}
		if createErr = database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); createErr != nil {
			return createErr
		}
		if strings.TrimSpace(req.Email) == "" {
			return nil
		}

		identity, createErr := serviceiamaccount.NewEmailIdentity(newUser.ID, req.Email)
		if createErr != nil {
			return createErr
		}
		return database.Database[*modeliamaccount.EmailIdentity](ctx).Create(identity)
	}); err != nil {
		// A rejected password is the caller's mistake, not the server's, and the
		// hashing helper already said which rule it broke. Its status and message
		// are rebuilt here rather than passed through, so this exit is one the
		// service constructed and the cause keeps its stack.
		var svcErr *service.Error
		if errors.As(err, &svcErr) {
			return nil, service.NewErrorWithCause(svcErr.Status(), svcErr.Msg(), err)
		}
		if errors.Is(err, database.ErrDuplicatedKey) {
			return nil, service.NewErrorWithCause(http.StatusConflict, "username or email already exists", err)
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to create user", err)
	}

	view, err := buildAdminUserView(ctx, newUser)
	if err != nil {
		return nil, err
	}

	log.Info("user created", "target_user_id", newUser.ID, "target_username", newUser.Username, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamuser.AdminUserCreateRsp{User: view}, nil
}
