package mfa

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authn"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/middleware/ratelimiter"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types/consts"
	"golang.org/x/time/rate"
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

	middleware.RegisterAuth(verificationRateLimiter())

	module.Use(module.NewWrapper("mfa/totp/bind", "id", false, &servicemfa.TOTPBindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/confirm", "id", false, &servicemfa.TOTPConfirmService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/totp/status", "id", false, &servicemfa.TOTPStatusService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("mfa/totp/unbind", "id", false, &servicemfa.TOTPUnbindService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("mfa/admin/users/:id/totp", "id", false, &servicemfa.AdminTOTPStatusService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("mfa/admin/users/:id/totp", "id", false, &servicemfa.AdminTOTPResetService{}), module.Exact(consts.PHASE_DELETE))
}

// verificationRateLimiter throttles the MFA endpoints that accept a guessable
// proof (a TOTP code or recovery code), per user and per endpoint: five
// attempts of burst with one attempt refilled every 12 seconds. Endpoints
// that accept no proof stay unthrottled.
func verificationRateLimiter() gin.HandlerFunc {
	return ratelimiter.RateLimiter(
		ratelimiter.WithRate(rate.Every(12*time.Second)),
		ratelimiter.WithBurst(5),
		ratelimiter.WithKeyFunc(func(c *gin.Context) string {
			return "mfa:" + c.FullPath() + ":" + c.GetString(consts.CTX_USER_ID)
		}),
		ratelimiter.WithSkipFunc(func(c *gin.Context) bool {
			switch c.FullPath() {
			case router.APIPathPrefix + "/mfa/totp/confirm",
				router.APIPathPrefix + "/mfa/totp/unbind":
				return false
			default:
				return true
			}
		}),
	)
}
