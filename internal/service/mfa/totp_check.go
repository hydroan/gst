package servicemfa

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// TOTPCheckService handles the public pre-login check for whether an account must
// complete a TOTP second-factor challenge. The service delegates primary
// credential verification to the configured AccountAuthenticator, then only reports
// the TOTP requirement for the authenticated account so callers cannot use the
// endpoint to enumerate which accounts have MFA enabled.
type TOTPCheckService struct {
	service.Base[*modelmfa.TOTPCheck, *modelmfa.TOTPCheckReq, *modelmfa.TOTPCheckRsp]
}

// Create validates the primary credentials and returns whether the matched
// account currently has any active TOTP devices. It does not issue login tokens or
// verify second-factor codes; it only tells the login flow whether a follow-up
// TOTP verification step is required.
func (c *TOTPCheckService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPCheckReq) (rsp *modelmfa.TOTPCheckRsp, err error) {
	log := c.WithServiceContext(ctx, ctx.GetPhase())

	// Validate input.
	if req.Username == "" {
		log.Warnw("empty username provided", "client_ip", ctx.ClientIP())
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		log.Warnw("empty password provided", "username", req.Username, "client_ip", ctx.ClientIP())
		return nil, errors.New("password is required")
	}

	account, err := currentAccountAuthenticator().AuthenticateByUsername(ctx, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrAccountAuthenticatorNotConfigured) {
			log.Errorw("mfa account authenticator is not configured", "username", req.Username, "error", err)
			return nil, newAccountAuthenticatorNotConfiguredServiceError(err)
		}
		if errors.Is(err, ErrAccountAuthenticationFailed) {
			log.Warnw("authentication failed", "username", req.Username, "client_ip", ctx.ClientIP(), "error", err)
			return nil, errors.New("authentication failed")
		}
		log.Errorw("failed to authenticate account", "username", req.Username, "error", err)
		return nil, errors.New("authentication failed")
	}
	if err = validateAuthenticatedAccount(account, ""); err != nil {
		log.Errorw("mfa account authenticator returned invalid account", "username", req.Username, "error", err)
		return nil, newAccountAuthenticatorInvalidAccountServiceError(err)
	}

	devices := make([]*modelmfa.TOTPDevice, 0)
	if err = database.Database[*modelmfa.TOTPDevice](ctx).
		WithQuery(&modelmfa.TOTPDevice{UserID: account.ID, IsActive: true}).
		List(&devices); err != nil {
		log.Errorw("failed to query TOTP devices", "user_id", account.ID, "error", err)
		return nil, errors.New("failed to check MFA status")
	}

	requiresMFA := len(devices) > 0

	// Log the check result.
	username := account.Username
	if username == "" {
		username = req.Username
	}
	log.Infow(
		"TOTP check completed",
		"username", username,
		"request_username", req.Username,
		"user_id", account.ID,
		"requires_mfa", requiresMFA,
		"active_devices", len(devices),
		"client_ip", ctx.ClientIP(),
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
