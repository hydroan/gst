package servicetwofa

import (
	"fmt"
	"net/http"

	"github.com/hydroan/gst/database"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
)

type TOTPUnbindService struct {
	service.Base[*modeltwofa.TOTPUnbind, *modeltwofa.TOTPUnbindReq, *modeltwofa.TOTPUnbindRsp]
}

func (t *TOTPUnbindService) Create(ctx *types.ServiceContext, req *modeltwofa.TOTPUnbindReq) (rsp *modeltwofa.TOTPUnbindRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	// 1. 验证用户身份
	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}

	// 2. 查找要解绑的设备
	device := &modeltwofa.TOTPDevice{}
	query := &modeltwofa.TOTPDevice{
		UserID:   ctx.UserID,
		IsActive: true,
	}
	query.Base.ID = req.DeviceID
	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithQuery(query).First(device); err != nil {
		log.Warnz("device not found or not active",
			zap.String("user_id", ctx.UserID),
			zap.String("device_id", req.DeviceID),
			zap.Error(err))
		return &modeltwofa.TOTPUnbindRsp{
			Success: false,
			Message: "Device not found or already unbound",
		}, nil
	}

	// 3. 如果提供了密码或TOTP码，进行额外验证
	if req.Password != "" {
		// TODO: 验证用户密码（需要根据实际的用户密码验证逻辑实现）
		log.Infoz("password verification requested", zap.String("user_id", ctx.UserID))
		// 这里应该调用用户密码验证服务
		// if !verifyUserPassword(ctx.UserID, req.Password) {
		//     return &TOTPUnbindRsp{
		//         Success: false,
		//         Message: "Invalid password",
		//     }, nil
		// }
	}

	if req.TOTPCode != "" {
		// 验证TOTP码
		valid := totp.Validate(req.TOTPCode, device.Secret)
		if !valid {
			log.Warnz("invalid totp code for unbind",
				zap.String("user_id", ctx.UserID),
				zap.String("device_id", req.DeviceID))
			return &modeltwofa.TOTPUnbindRsp{
				Success: false,
				Message: "Invalid TOTP code",
			}, nil
		}
		log.Infoz("totp code validated for unbind", zap.String("user_id", ctx.UserID))
	}

	// 4. 删除设备
	device.IsActive = false
	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithPurge(true).Delete(device); err != nil {
		log.Errorz("failed to delete device status",
			zap.String("user_id", ctx.UserID),
			zap.String("device_id", req.DeviceID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unbind device: %w", err)
	}

	log.Infoz("totp device unbound successfully",
		zap.String("user_id", ctx.UserID),
		zap.String("device_id", req.DeviceID),
		zap.String("device_name", device.DeviceName))

	// 5. 统计剩余活跃设备数量
	activeDevices := make([]*modeltwofa.TOTPDevice, 0)
	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithQuery(&modeltwofa.TOTPDevice{
		UserID:   ctx.UserID,
		IsActive: true,
	}).List(&activeDevices); err != nil {
		log.Errorz("failed to count active devices",
			zap.String("user_id", ctx.UserID),
			zap.Error(err))
		// 不返回错误，因为解绑操作已经成功
	}

	deviceCount := len(activeDevices)
	log.Infoz("active device count after unbind",
		zap.String("user_id", ctx.UserID),
		zap.Int("count", deviceCount))

	// 6. 返回操作结果
	rsp = &modeltwofa.TOTPUnbindRsp{
		Success:     true,
		Message:     fmt.Sprintf("Device '%s' unbound successfully", device.DeviceName),
		DeviceCount: deviceCount,
	}

	return rsp, nil
}
