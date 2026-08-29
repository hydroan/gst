package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware/ratelimiter"
	"github.com/hydroan/gst/types/consts"
	"golang.org/x/time/rate"
)

// MFAVerificationRateLimit throttles the MFA endpoints that accept a guessable
// proof (a TOTP code or recovery code), per user and per endpoint: five
// attempts of burst with one attempt refilled every 12 seconds. Endpoints that
// accept no proof stay unthrottled.
//
// It lives here rather than inside module/mfa so the add path and the copy path
// register the same handler: gg module copy carries middleware declared in
// module.json into the project and wires it into middleware/middleware.go,
// while a handler hidden in a module package would silently be add-only.
func MFAVerificationRateLimit() gin.HandlerFunc {
	return ratelimiter.RateLimiter(
		ratelimiter.WithRate(rate.Every(12*time.Second)),
		ratelimiter.WithBurst(5),
		ratelimiter.WithKeyFunc(func(c *gin.Context) string {
			return "mfa:" + c.FullPath() + ":" + c.GetString(consts.CTX_USER_ID)
		}),
		ratelimiter.WithSkipFunc(func(c *gin.Context) bool {
			switch c.FullPath() {
			case consts.APIPathPrefix + "/mfa/totp/confirm",
				consts.APIPathPrefix + "/mfa/totp/unbind":
				return false
			default:
				return true
			}
		}),
	)
}
