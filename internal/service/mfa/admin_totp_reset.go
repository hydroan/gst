package servicemfa

import (
	"context"
	"net/http"
	"strings"

	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// AdminTOTPResetService force-clears a target account's TOTP enrollment.
//
// This is the rescue path for an account that lost both its authenticator
// device and its recovery codes: self-service unbind deliberately accepts no
// password, so only an administrator can switch MFA off for such an account.
// The reset hard-deletes every device — active or still staged — so no shared
// secret or recovery-code hash survives, and the account logs in without a
// second factor until it enrolls again.
type AdminTOTPResetService struct {
	service.Base[*modelmfa.AdminTOTP, *model.Empty, *modelmfa.AdminTOTPResetRsp]
}

// Delete removes every TOTP device of the target user on behalf of an administrator.
func (a *AdminTOTPResetService) Delete(ctx *types.ServiceContext, req *model.Empty) (rsp *modelmfa.AdminTOTPResetRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())

	targetUserID := strings.TrimSpace(ctx.Param("id"))
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}
	if err = currentAccountAdministrator().EnsureCanAdminister(ctx, targetUserID); err != nil {
		return nil, err
	}

	removed := 0
	err = database.Transaction(ctx, func(ctx context.Context) error {
		devices := make([]*modelmfa.TOTPDevice, 0)
		if listErr := database.Database[*modelmfa.TOTPDevice](ctx).WithLock(consts.LockUpdate).WithQuery(&modelmfa.TOTPDevice{
			UserID: targetUserID,
		}).List(&devices); listErr != nil {
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to list TOTP devices", listErr)
		}
		if len(devices) == 0 {
			return nil
		}
		if deleteErr := database.Database[*modelmfa.TOTPDevice](ctx).WithPurge(true).Delete(devices...); deleteErr != nil {
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to reset TOTP devices", deleteErr)
		}
		removed = len(devices)
		return nil
	})
	if err != nil {
		log.Errorz("failed to reset totp enrollment",
			zap.String("actor_user_id", ctx.UserID()),
			zap.String("target_user_id", targetUserID),
			zap.Error(err))
		return nil, err
	}

	log.Infoz("totp enrollment reset by administrator",
		zap.String("actor_user_id", ctx.UserID()),
		zap.String("target_user_id", targetUserID),
		zap.Int("removed_device_count", removed))

	return &modelmfa.AdminTOTPResetRsp{RemovedDeviceCount: removed}, nil
}
