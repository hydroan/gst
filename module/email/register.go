package email

import (
	serviceemail "github.com/hydroan/gst/internal/service/email"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers email verification, password reset, and email change routes.
//
// Routes:
//   - POST /api/iam/email/verification-request
//   - POST /api/iam/email/verification-resend
//   - POST /api/iam/email/verification-confirm
//   - POST /api/iam/email/password-reset-request
//   - POST /api/iam/email/password-reset-confirm
//   - POST /api/iam/email/change-request
//   - POST /api/iam/email/change-resend
//   - POST /api/iam/email/change-cancel
//   - POST /api/iam/email/change-confirm
func Register() {
	serviceemail.SetAccountGateway(iamAccountGateway{})

	module.Use(module.NewWrapper("/iam/email/verification-request", "id", true, &serviceemail.VerificationRequestService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/verification-resend", "id", true, &serviceemail.VerificationResendService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/verification-confirm", "id", true, &serviceemail.VerificationConfirmService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/password-reset-request", "id", true, &serviceemail.PasswordResetRequestService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/password-reset-confirm", "id", true, &serviceemail.PasswordResetConfirmService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/change-request", "id", false, &serviceemail.ChangeRequestService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/change-resend", "id", false, &serviceemail.ChangeResendService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/change-cancel", "id", true, &serviceemail.ChangeCancelService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/email/change-confirm", "id", false, &serviceemail.ChangeConfirmService{}), module.CRUD(consts.PHASE_CREATE))
}
