/*
用户登录流程：
1. POST /api/login → 普通登录
2. POST /api/2fa/totp/verify → 验证TOTP码（如果启用2FA）
3. 登录成功

2FA管理流程：
1. POST /api/2fa/totp/check → 检查用户是否启用2FA
2. POST /api/2fa/totp/bind → 绑定设备
3. POST /api/2fa/totp/confirm → 确认绑定
4. POST /api/2fa/totp/verify → 日常验证使用 ⭐
5. POST /api/2fa/totp/unbind → 解绑设备
6. GET /api/2fa/totp/status → 查看状态



核心接口

绑定流程：
- POST /api/2fa/totp/bind - 初始化 TOTP 绑定
- POST /api/2fa/totp/confirm - 确认绑定 TOTP 设备

验证流程：
- POST /api/2fa/totp/verify - 验证 TOTP 代码

检查接口：
- POST /api/2fa/totp/check - 检查用户是否启用 2FA

管理接口：
- GET /api/2fa/totp/status - 获取用户 2FA 状态
- POST /api/2fa/totp/unbind - 解绑 TOTP 设备
- GET /api/2fa/totp/devices - 获取设备列表
- DELETE /api/2fa/totp/devices/:id - 删除设备



核心服务逻辑

A. TOTP 绑定服务
- 生成随机密钥
- 创建 QR 码 URL
- 生成备份码
- 返回绑定信息 B. TOTP 确认服务
- 验证用户输入的 TOTP 代码
- 保存设备信息到数据库
- 激活 2FA 功能

B. TOTP 确认服务
- 验证用户输入的 TOTP 代码
- 保存设备信息到数据库
- 激活 2FA 功能

C. TOTP 验证服务
- 验证 TOTP 代码或备份码
- 更新设备使用时间
- 返回验证结果

D. TOTP 解绑服务
- 验证用户身份和权限
- 查找并软删除指定设备
- 统计剩余活跃设备数量
- 返回解绑结果

E. TOTP 检查服务
- 验证用户身份（用户名和密码）
- 查询用户的活跃 TOTP 设备
- 返回是否启用 2FA 的状态

F. TOTP 状态服务
- 查询用户的 2FA 状态
- 返回设备列表信息
*/

package twofa

import (
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers the models: TOTPBind, TOTPCheck, TOTPConfirm, TOTPDevice, TOTPStatus, TOTPUnbind and TOTPVerify.
//
// Modules, Payload and Result:
//   - TOTPBind, TOTPBindRsp
//   - TOTPCheck, TOTPCheckReq, TOTPCheckRsp
//   - TOTPConfirm, TOTPConfirmReq, TOTPConfirmRsp
//   - TOTPDevice
//   - TOTPStatus, TOTPStatusRsp
//   - TOTPUnbind, TOTPUnbindReq, TOTPUnbindRsp
//   - TOTPVerify, TOTPVerifyReq, TOTPVerifyRsp
//
// Routes
//   - POST     /api/2fa/totp/bind
//   - POST     /api/2fa/totp/check
//   - POST     /api/2fa/totp/confirm
//   - POST     /api/2fa/totp/status
//   - POST     /api/2fa/totp/unbind
//   - POST     /api/2fa/totp/verify
//   - POST     /api/2fa/totp/devices
//   - DELETE   /api/2fa/totp/devices/:id
//   - PUT      /api/2fa/totp/devices/:id
//   - PATCH    /api/2fa/totp/devices/:id
//   - GET      /api/2fa/totp/devices
//   - GET      /api/2fa/totp/devices/:id
func Register() {
	servicetwofa.Enabled = true

	module.Use[
		*TOTPBind,
		*TOTPBind,
		*TOTPBindRsp](
		&TOTPBindModule{},
		consts.PHASE_CREATE,
	)

	module.Use[
		*TOTPCheck,
		*TOTPCheckReq,
		*TOTPCheckRsp](
		&TOTPCheckModule{},
		consts.PHASE_CREATE,
	)

	module.Use[
		*TOTPConfirm,
		*TOTPConfirmReq,
		*TOTPConfirmRsp](
		&TOTPConfirmModule{},
		consts.PHASE_CREATE,
	)

	module.Use[
		*TOTPDevice,
		*TOTPDevice,
		*TOTPDevice](
		&TOTPDeviceModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*TOTPStatus,
		*TOTPStatus,
		*TOTPStatusRsp](
		&TOTPStatusModule{},
		consts.PHASE_LIST,
	)

	module.Use[
		*TOTPUnbind,
		*TOTPUnbindReq,
		*TOTPUnbindRsp](
		&TOTPUnbindModule{},
		consts.PHASE_CREATE,
	)

	module.Use[
		*TOTPVerify,
		*TOTPVerifyReq,
		*TOTPVerifyRsp](
		&TOTPVerifyModule{},
		consts.PHASE_CREATE,
	)
}
