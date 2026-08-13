package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register wires TOTP-based MFA into the application.
//
// Besides registering the routes below and the internal TOTPDevice table, it
// turns servicemfa.Enabled on and installs the framework IAM account store as
// the AccountAuthenticator behind password-based MFA flows. Projects using
// copied MFA source install their own AccountAuthenticator from project-owned
// code instead of editing service/mfa.
//
// Routes:
//   - POST /api/mfa/totp/bind
//   - POST /api/mfa/totp/check (public)
//   - POST /api/mfa/totp/confirm
//   - GET  /api/mfa/totp/status
//   - POST /api/mfa/totp/unbind
//   - POST /api/mfa/totp/verify
func Register() {
	servicemfa.Enabled = true
	servicemfa.SetAccountAuthenticator(iamAccountAuthenticator{})
	model.Register[*modelmfa.TOTPDevice]()

	module.Use(module.NewWrapper("mfa/totp/bind", "id", false, &servicemfa.TOTPBindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/check", "id", true, &servicemfa.TOTPCheckService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/confirm", "id", false, &servicemfa.TOTPConfirmService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/status", "id", false, &servicemfa.TOTPStatusService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("mfa/totp/unbind", "id", false, &servicemfa.TOTPUnbindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/verify", "id", false, &servicemfa.TOTPVerifyService{}), module.CRUD(consts.PHASE_CREATE))
}
