package servicemfa

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// TOTPStatusService returns the current authenticated account's TOTP enrollment
// state. The service keeps the status view scoped to ctx.UserID(), counts only
// active devices as enabling MFA, and returns device metadata without exposing
// secrets or recovery-code hashes.
//
// The request type is *model.Empty to match the DSL: a List action that only
// declares a Result always parses with an empty payload, and the add path must
// register the same request type the copy path generates.
type TOTPStatusService struct {
	service.Base[*modelmfa.TOTPStatus, *model.Empty, *modelmfa.TOTPStatusRsp]
}

// List loads the current user's TOTP devices and builds the status response
// used by clients to render MFA settings. It requires an authenticated request,
// returns active devices only, and derives Enabled from the active device count.
func (t *TOTPStatusService) List(ctx *types.ServiceContext, req *model.Empty) (rsp *modelmfa.TOTPStatusRsp, err error) {
	log := t.WithContext(ctx, ctx.Phase())

	// 1. Verify the authenticated account.
	if len(ctx.UserID()) == 0 {
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}

	// 2. Build the status view scoped to the current user.
	rsp, err = buildTOTPStatusRsp(ctx, ctx.UserID())
	if err != nil {
		return nil, err
	}

	log.Infoz("totp status retrieved successfully",
		zap.String("user_id", ctx.UserID()),
		zap.Int("active_devices", rsp.DeviceCount),
		zap.Bool("enabled", rsp.Enabled))

	return rsp, nil
}

// buildTOTPStatusRsp loads one account's active TOTP devices and renders the
// sanitized enrollment view shared by the self-service and administrative
// status endpoints: device metadata only, never secrets or recovery-code
// hashes.
func buildTOTPStatusRsp(ctx *types.ServiceContext, userID string) (*modelmfa.TOTPStatusRsp, error) {
	devices := make([]*modelmfa.TOTPDevice, 0)
	if err := database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(&modelmfa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}).List(&devices); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to retrieve device information", err)
	}

	deviceInfos := make([]modelmfa.TOTPDeviceInfo, 0, len(devices))
	for _, device := range devices {
		deviceInfos = append(deviceInfos, modelmfa.TOTPDeviceInfo{
			ID:         device.ID,
			DeviceName: device.DeviceName,
			LastUsedAt: device.LastUsedAt,
			CreatedAt:  device.CreatedAt,
		})
	}

	return &modelmfa.TOTPStatusRsp{
		Enabled:     len(devices) > 0,
		DeviceCount: len(devices),
		Devices:     deviceInfos,
	}, nil
}
