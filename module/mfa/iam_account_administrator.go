package mfa

import (
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/types"
)

// iamAccountAdministrator authorizes administrative MFA operations through the
// framework IAM tenant-admin rules. It lives under module/mfa so copied MFA
// service code does not import framework IAM internals; copied projects
// install their own AccountAdministrator from project-owned code.
type iamAccountAdministrator struct{}

func (iamAccountAdministrator) EnsureCanAdminister(ctx *types.ServiceContext, targetUserID string) error {
	// EnsureTenantAdmin grants system-root actors globally. For tenant admins
	// it first checks route permission in the current tenant, then checks that
	// the target user is a member of that same tenant; system-root targets are
	// never manageable through tenant-local admin APIs.
	actor, target, err := serviceiamaccount.LoadActorAndTarget(ctx, targetUserID)
	if err != nil {
		return err
	}
	return adminauth.EnsureTenantAdmin(ctx, actor, target)
}
