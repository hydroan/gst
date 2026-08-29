package mfa

import (
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/types"
)

// iamAccountAdministrator authorizes administrative MFA operations through the
// framework IAM tenant-admin rules. It lives under module/mfa so copied MFA
// service code does not import framework IAM internals; copied projects
// install their own AccountAdministrator from project-owned code.
type iamAccountAdministrator struct{}

func (iamAccountAdministrator) EnsureCanAdminister(ctx *types.ServiceContext, targetUserID string) error {
	return iam.EnsureAdminOnUser(ctx, targetUserID)
}
