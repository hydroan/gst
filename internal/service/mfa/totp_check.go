package servicemfa

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
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
	log := c.WithContext(ctx, ctx.Phase())

	// Validate input.
	if req.Username == "" {
		return nil, service.NewError(http.StatusBadRequest, "username is required")
	}
	if req.Password == "" {
		return nil, service.NewError(http.StatusBadRequest, "password is required")
	}

	account, err := currentAccountAuthenticator().AuthenticateByUsername(ctx, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrAccountAuthenticatorNotConfigured) {
			log.Errorz("mfa account authenticator is not configured", zap.String("username", req.Username), zap.Error(err))
			return nil, newAccountAuthenticatorNotConfiguredServiceError(err)
		}
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "authentication failed", err)
	}
	if err = validateAuthenticatedAccount(account, ""); err != nil {
		log.Errorz("mfa account authenticator returned invalid account", zap.String("username", req.Username), zap.Error(err))
		return nil, newAccountAuthenticatorInvalidAccountServiceError(err)
	}

	devices := make([]*modelmfa.TOTPDevice, 0)
	if err = database.Database[*modelmfa.TOTPDevice](ctx).
		WithQuery(&modelmfa.TOTPDevice{UserID: account.ID, IsActive: true}).
		List(&devices); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to check MFA status", err)
	}

	requiresMFA := len(devices) > 0

	// Log the check result.
	username := account.Username
	if username == "" {
		username = req.Username
	}
	log.Infoz(
		"TOTP check completed",
		zap.String("username", username),
		zap.String("request_username", req.Username),
		zap.String("user_id", account.ID),
		zap.Bool("requires_mfa", requiresMFA),
		zap.Int("active_devices", len(devices)),
		zap.String("client_ip", ctx.ClientIP()),
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
