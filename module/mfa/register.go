package mfa

import (
	"github.com/hydroan/gst/authn"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register wires TOTP-based MFA into the application.
//
// Besides registering the routes below and the internal TOTPDevice table, it
// throttles the endpoints that accept a guessable proof, installs the
// framework IAM tenant-admin rules as the AccountAdministrator behind the
// administrative routes, and installs the login second-factor gate. Every one
// of those is an explicit call below: nothing arms itself through a package
// import, so a copied module reproduces this list from project-owned assembly
// code instead of inheriting hidden initialization.
//
// Routes:
//   - POST   /api/mfa/totp/bind
//   - POST   /api/mfa/totp/confirm
//   - GET    /api/mfa/totp/status
//   - POST   /api/mfa/totp/unbind
//   - GET    /api/mfa/admin/users/:id/totp
//   - DELETE /api/mfa/admin/users/:id/totp
func Register() {
	servicemfa.SetAccountAdministrator(iamAccountAdministrator{})
	authn.SetLoginSecondFactorVerifier(servicemfa.LoginSecondFactorVerifier)
	model.Register[*modelmfa.TOTPDevice]()

	middleware.RegisterAuth(middleware.MFAVerificationRateLimit())

	module.Use(module.NewWrapper("mfa/totp/bind", "id", false, &servicemfa.TOTPBindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/confirm", "id", false, &servicemfa.TOTPConfirmService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/status", "id", false, &servicemfa.TOTPStatusService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("mfa/totp/unbind", "id", false, &servicemfa.TOTPUnbindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/admin/users/:id/totp", "id", false, &servicemfa.AdminTOTPStatusService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("mfa/admin/users/:id/totp", "id", false, &servicemfa.AdminTOTPResetService{}), module.Exact(consts.PHASE_DELETE))
}
