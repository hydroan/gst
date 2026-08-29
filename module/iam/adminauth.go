package iam

import (
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/types"
)

// EnsureAdminOnUser reports whether the acting user may administer the target
// user under the IAM tenant-admin rules, returning a service error when it may
// not. System-root actors are granted globally; a tenant administrator must
// hold route permission in the current tenant and the target must belong to
// that same tenant, and system-root targets are never manageable through
// tenant-local admin APIs.
//
// This is the seam other framework modules use to reuse IAM authorization.
// Resolving the two users and evaluating the rules are IAM's own steps, so a
// caller states what it wants to know rather than reproducing the procedure —
// and IAM stays free to change the procedure without touching those modules.
func EnsureAdminOnUser(ctx *types.ServiceContext, targetUserID string) error {
	actor, target, err := serviceiamaccount.LoadActorAndTarget(ctx, targetUserID)
	if err != nil {
		return err
	}

	return adminauth.EnsureTenantAdmin(ctx, actor, target)
}
