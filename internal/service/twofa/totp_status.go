package servicetwofa

import (
	"fmt"
	"net/http"

	"github.com/hydroan/gst/database"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// TOTPStatusService returns the current authenticated user's TOTP enrollment
// state. The service keeps the status view scoped to ctx.UserID, counts only
// active devices as enabling MFA, and returns device metadata without exposing
// secrets or recovery-code hashes.
type TOTPStatusService struct {
	service.Base[*modeltwofa.TOTPStatus, *modeltwofa.TOTPStatus, *modeltwofa.TOTPStatusRsp]
}

// List loads the current user's TOTP devices and builds the status response
// used by clients to render MFA settings. It requires an authenticated request,
// includes inactive devices for management visibility, and derives Enabled from
// the number of active devices.
func (t *TOTPStatusService) List(ctx *types.ServiceContext, req *modeltwofa.TOTPStatus) (rsp *modeltwofa.TOTPStatusRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	// 1. 验证用户身份
	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}

	// 2. 查询用户的所有 TOTP 设备
	devices := make([]*modeltwofa.TOTPDevice, 0)
	query := &modeltwofa.TOTPDevice{
		UserID: ctx.UserID,
	}

	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithQuery(query).List(&devices); err != nil {
		log.Errorz("failed to list totp devices", zap.Error(err))
		return nil, fmt.Errorf("failed to retrieve device information: %w", err)
	}

	// 3. 统计设备状态和转换设备信息
	activeDeviceCount := 0
	deviceInfos := make([]modeltwofa.TOTPDeviceInfo, 0, len(devices))

	for _, device := range devices {
		// 统计活跃设备数量
		if device.IsActive {
			activeDeviceCount++
		}

		// 转换设备信息（不包含敏感信息）
		deviceInfo := modeltwofa.TOTPDeviceInfo{
			ID:         device.ID,
			DeviceName: device.DeviceName,
			IsActive:   device.IsActive,
			CreatedAt:  device.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), // RFC3339 格式
		}

		// 格式化最后使用时间
		if device.LastUsedAt != nil {
			lastUsedStr := device.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
			deviceInfo.LastUsedAt = &lastUsedStr
		}

		deviceInfos = append(deviceInfos, deviceInfo)
	}

	// 4. 构建响应
	rsp = &modeltwofa.TOTPStatusRsp{
		Enabled:     activeDeviceCount > 0, // 有活跃设备则启用 2FA
		DeviceCount: activeDeviceCount,
		Devices:     deviceInfos,
	}

	log.Infoz("totp status retrieved successfully",
		zap.String("user_id", ctx.UserID),
		zap.Int("total_devices", len(devices)),
		zap.Int("active_devices", activeDeviceCount),
		zap.Bool("enabled", rsp.Enabled))

	return rsp, nil
}
