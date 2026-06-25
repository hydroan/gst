package servicemfa

import (
	"fmt"
	"net/http"

	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// TOTPStatusService returns the current authenticated account's TOTP enrollment
// state. The service keeps the status view scoped to ctx.UserID(), counts only
// active devices as enabling MFA, and returns device metadata without exposing
// secrets or recovery-code hashes.
type TOTPStatusService struct {
	service.Base[*modelmfa.TOTPStatus, *modelmfa.TOTPStatus, *modelmfa.TOTPStatusRsp]
}

// List loads the current user's TOTP devices and builds the status response
// used by clients to render MFA settings. It requires an authenticated request,
// returns active devices only, and derives Enabled from the active device count.
func (t *TOTPStatusService) List(ctx *types.ServiceContext, req *modelmfa.TOTPStatus) (rsp *modelmfa.TOTPStatusRsp, err error) {
	log := t.WithContext(ctx, ctx.GetPhase())

	// 1. Verify the authenticated account.
	if len(ctx.UserID()) == 0 {
		log.Errorz("user_id not found in context")
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}

	// 2. Load active TOTP devices for the user.
	devices := make([]*modelmfa.TOTPDevice, 0)
	query := &modelmfa.TOTPDevice{
		UserID:   ctx.UserID(),
		IsActive: true,
	}

	if err = database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(query).List(&devices); err != nil {
		log.Errorz("failed to list totp devices", zap.Error(err))
		return nil, fmt.Errorf("failed to retrieve device information: %w", err)
	}

	// 3. Count device states and build the public device view.
	activeDeviceCount := len(devices)
	deviceInfos := make([]modelmfa.TOTPDeviceInfo, 0, len(devices))

	for _, device := range devices {
		// Convert device metadata without sensitive fields.
		deviceInfo := modelmfa.TOTPDeviceInfo{
			ID:         device.ID,
			DeviceName: device.DeviceName,
			CreatedAt:  device.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), // RFC3339 format
		}

		// Format the last-used timestamp when present.
		if device.LastUsedAt != nil {
			lastUsedStr := device.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
			deviceInfo.LastUsedAt = &lastUsedStr
		}

		deviceInfos = append(deviceInfos, deviceInfo)
	}

	// 4. Build the response.
	rsp = &modelmfa.TOTPStatusRsp{
		Enabled:     activeDeviceCount > 0, // Active devices enable MFA
		DeviceCount: activeDeviceCount,
		Devices:     deviceInfos,
	}

	log.Infoz("totp status retrieved successfully",
		zap.String("user_id", ctx.UserID()),
		zap.Int("total_devices", len(devices)),
		zap.Int("active_devices", activeDeviceCount),
		zap.Bool("enabled", rsp.Enabled))

	return rsp, nil
}
