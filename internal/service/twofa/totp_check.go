package servicetwofa

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

type TOTPCheckService struct {
	service.Base[*modeltwofa.TOTPCheck, *modeltwofa.TOTPCheckReq, *modeltwofa.TOTPCheckRsp]
}

func (c *TOTPCheckService) Create(ctx *types.ServiceContext, req *modeltwofa.TOTPCheckReq) (rsp *modeltwofa.TOTPCheckRsp, err error) {
	log := c.WithServiceContext(ctx, ctx.GetPhase())

	// 验证输入参数
	if req.Username == "" {
		log.Warnw("empty username provided", "client_ip", ctx.ClientIP)
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		log.Warnw("empty password provided", "username", req.Username, "client_ip", ctx.ClientIP)
		return nil, errors.New("password is required")
	}

	// 查找用户
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

	// 验证密码
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Warnw("invalid password", "username", req.Username, "client_ip", ctx.ClientIP)
		return nil, errors.New("authentication failed")
	}

	// 检查用户是否有活跃的TOTP设备
	totpDB := database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext())
	devices := make([]*modeltwofa.TOTPDevice, 0)
	if err = totpDB.WithQuery(&modeltwofa.TOTPDevice{UserID: user.ID, IsActive: true}).List(&devices); err != nil {
		log.Errorw("failed to query TOTP devices", "user_id", user.ID, "error", err)
		return nil, errors.New("failed to check 2FA status")
	}

	requires2FA := len(devices) > 0

	// 记录检查日志
	log.Infow(
		"TOTP check completed",
		"username", req.Username,
		"user_id", user.ID,
		"requires_2fa", requires2FA,
		"active_devices", len(devices),
		"client_ip", ctx.ClientIP,
	)

	// 返回检查结果
	message := "2FA is not enabled"
	if requires2FA {
		message = "2FA is enabled"
	}

	return &modeltwofa.TOTPCheckRsp{
		Requires2FA: requires2FA,
		Message:     message,
	}, nil
}
