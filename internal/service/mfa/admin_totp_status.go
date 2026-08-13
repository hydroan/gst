package servicemfa

import (
	"net/http"
	"strings"

	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// AdminTOTPStatusService returns a target account's TOTP enrollment state to
// an administrator. Authorization is delegated to the installed
// AccountAdministrator before any enrollment data is read, and the response is
// the same sanitized view the self-service status endpoint renders: device
// metadata only, never secrets or recovery-code hashes.
type AdminTOTPStatusService struct {
	service.Base[*modelmfa.AdminTOTP, *model.Empty, *modelmfa.TOTPStatusRsp]
}

// Get loads the target user's TOTP enrollment view for an administrator.
func (a *AdminTOTPStatusService) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *modelmfa.TOTPStatusRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())

	targetUserID := strings.TrimSpace(ctx.Param("id"))
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}
	if err = currentAccountAdministrator().EnsureCanAdminister(ctx, targetUserID); err != nil {
		return nil, err
	}

	rsp, err = buildTOTPStatusRsp(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	log.Infoz("admin totp status retrieved",
		zap.String("actor_user_id", ctx.UserID()),
		zap.String("target_user_id", targetUserID),
		zap.Bool("enabled", rsp.Enabled))

	return rsp, nil
}
