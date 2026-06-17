package servicetwofa

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
)

type TOTPConfirmService struct {
	service.Base[*modeltwofa.TOTPConfirm, *modeltwofa.TOTPConfirmReq, *modeltwofa.TOTPConfirmRsp]
}

func (t *TOTPConfirmService) Create(ctx *types.ServiceContext, req *modeltwofa.TOTPConfirmReq) (rsp *modeltwofa.TOTPConfirmRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	// 1. 验证用户身份
	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}

	// 2. 验证 secret 格式（Base32 编码，通常32字符）
	if len(req.Secret) == 0 {
		log.Errorz("secret is empty")
		return nil, errors.New("secret is required")
	}

	// // 验证 secret 是否为有效的 Base32 格式
	// if len(req.Secret) != 32 {
	// 	log.Errorz("invalid secret length", zap.Int("length", len(req.Secret)))
	// 	return nil, fmt.Errorf("invalid secret format")
	// }

	// 3. 验证 TOTP 代码
	valid := totp.Validate(req.Code, req.Secret)
	if !valid {
		log.Warnz("invalid totp code", zap.String("user_id", ctx.UserID))
		return nil, errors.New("invalid TOTP code")
	}

	log.Infoz("totp code validated successfully", zap.String("user_id", ctx.UserID))

	// 4. 检查是否已存在相同 secret 的设备（防止重复绑定）
	devices := make([]*modeltwofa.TOTPDevice, 0)
	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithQuery(&modeltwofa.TOTPDevice{
		UserID: ctx.UserID,
		Secret: req.Secret,
	}).WithLimit(1).List(&devices); err != nil {
		log.Errorz("failed to list devices", zap.Error(err))
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	if len(devices) > 0 {
		log.Warnz("device already exists", zap.String("user_id", ctx.UserID), zap.String("device_id", devices[0].ID))
		return nil, errors.New("device already bound")
	}

	// 5. 生成备份码
	backupCodes, err := generateBackupCodes()
	if err != nil {
		log.Errorz("failed to generate backup codes", zap.Error(err))
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// 6. 创建 TOTP 设备记录
	now := time.Now()
	device := &modeltwofa.TOTPDevice{
		UserID:      ctx.UserID,
		DeviceName:  req.DeviceName,
		Secret:      req.Secret,
		BackupCodes: backupCodes,
		IsActive:    true,
		LastUsedAt:  &now,
	}

	if err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).Create(device); err != nil {
		log.Errorz("failed to create totp device", zap.Error(err))
		return nil, fmt.Errorf("failed to save device: %w", err)
	}

	log.Infoz("totp device created successfully",
		zap.String("user_id", ctx.UserID),
		zap.String("device_id", device.ID))

	// 8. 返回响应
	rsp = &modeltwofa.TOTPConfirmRsp{
		DeviceID:    device.ID,
		Message:     "TOTP device confirmed and activated successfully",
		BackupCodes: backupCodes,
	}

	return rsp, nil
}

// generateBackupCodes generates 8 backup codes, each of 8 numeric digits.
//
//lint:ignore modernize Keep classic loops and string concatenation for explicit clarity.
func generateBackupCodes() ([]string, error) {
	codes := make([]string, 8)
	for i := range 8 {
		// 生成8位随机数字
		var b strings.Builder
		b.Grow(8)
		for range 8 {
			digit, err := rand.Int(rand.Reader, big.NewInt(10))
			if err != nil {
				return nil, fmt.Errorf("failed to generate random digit: %w", err)
			}
			b.WriteByte('0' + byte(digit.Int64())) //nolint:gosec // G115: d is explicitly validated to be in [0,9] before conversion
		}
		codes[i] = b.String()
	}
	return codes, nil
}
