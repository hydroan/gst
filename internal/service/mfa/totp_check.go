package servicemfa

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// TOTPCheckService handles the public pre-login check for whether a user must
// complete a TOTP second-factor challenge. The service first verifies the
// supplied username and password, then only reports the TOTP requirement for
// the authenticated account so callers cannot use the endpoint to enumerate
// which users have MFA enabled.
type TOTPCheckService struct {
	service.Base[*modelmfa.TOTPCheck, *modelmfa.TOTPCheckReq, *modelmfa.TOTPCheckRsp]
}

// Create validates the primary credentials and returns whether the matched
// user currently has any active TOTP devices. It does not issue login tokens or
// verify second-factor codes; it only tells the login flow whether a follow-up
// TOTP verification step is required.
func (c *TOTPCheckService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPCheckReq) (rsp *modelmfa.TOTPCheckRsp, err error) {
	log := c.WithServiceContext(ctx, ctx.GetPhase())

	// Validate input.
	if req.Username == "" {
		log.Warnw("empty username provided", "client_ip", ctx.ClientIP)
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		log.Warnw("empty password provided", "username", req.Username, "client_ip", ctx.ClientIP)
		return nil, errors.New("password is required")
	}

	// Find the user.
	db := database.Database[*modeliamuser.User](ctx.DatabaseContext())
	users := make([]*modeliamuser.User, 0)
	if err = db.WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&users); err != nil {
		log.Errorw("failed to query user", "username", req.Username, "error", err)
		return nil, errors.New("authentication failed")
	}
	if len(users) == 0 {
		log.Warnw("user not found", "username", req.Username, "client_ip", ctx.ClientIP)
		return nil, errors.New("authentication failed")
	}
	user := users[0]

	// Verify the password.
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Warnw("invalid password", "username", req.Username, "client_ip", ctx.ClientIP)
		return nil, errors.New("authentication failed")
	}

	// Check whether the user has active TOTP devices.
	totpDB := database.Database[*modelmfa.TOTPDevice](ctx.DatabaseContext())
	devices := make([]*modelmfa.TOTPDevice, 0)
	if err = totpDB.WithQuery(&modelmfa.TOTPDevice{UserID: user.ID, IsActive: true}).List(&devices); err != nil {
		log.Errorw("failed to query TOTP devices", "user_id", user.ID, "error", err)
		return nil, errors.New("failed to check MFA status")
	}

	requiresMFA := len(devices) > 0

	// Log the check result.
	log.Infow(
		"TOTP check completed",
		"username", req.Username,
		"user_id", user.ID,
		"requires_mfa", requiresMFA,
		"active_devices", len(devices),
		"client_ip", ctx.ClientIP,
	)

	// Return the check result.
	message := "MFA is not enabled"
	if requiresMFA {
		message = "MFA is enabled"
	}

	return &modelmfa.TOTPCheckRsp{
		RequiresMFA: requiresMFA,
		Message:     message,
	}, nil
}
