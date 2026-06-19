/*
用户登录流程：
1. POST /api/login → 普通登录
2. 登录时通过 totp_code 或 backup_code 完成 2FA 验证（如果启用2FA）
3. 登录成功

2FA管理流程：
1. POST /api/2fa/totp/check → 检查用户是否启用2FA
2. POST /api/2fa/totp/bind → 绑定设备
3. POST /api/2fa/totp/confirm → 确认绑定
4. POST /api/2fa/totp/verify → 日常验证使用
5. POST /api/2fa/totp/unbind → 解绑设备
6. GET /api/2fa/totp/status → 查看状态和设备摘要

核心接口

绑定流程：
- POST /api/2fa/totp/bind - 初始化 TOTP 绑定
- POST /api/2fa/totp/confirm - 确认绑定 TOTP 设备

验证流程：
- POST /api/2fa/totp/verify - 验证 TOTP 代码

检查接口：
- POST /api/2fa/totp/check - 检查用户是否启用 2FA

管理接口：
- GET /api/2fa/totp/status - 获取用户 2FA 状态和脱敏设备摘要
- POST /api/2fa/totp/unbind - 解绑 TOTP 设备

核心服务逻辑

A. TOTP 绑定服务
- 生成随机密钥
- 创建 QR 码 URL
- 创建待确认绑定挑战
- 返回挑战 ID 和绑定信息

B. TOTP 确认服务
- 验证用户输入的 TOTP 代码
- 生成一次性恢复码
- 保存设备信息到数据库
- 激活 2FA 功能

C. TOTP 验证服务
- 验证 TOTP 代码或恢复码
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
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers TOTP routes and the internal TOTP device table.
//
// Modules, Payload and Result:
//   - TOTPBind, TOTPBindRsp
//   - TOTPCheck, TOTPCheckReq, TOTPCheckRsp
//   - TOTPConfirm, TOTPConfirmReq, TOTPConfirmRsp
//   - TOTPStatus, TOTPStatusRsp
//   - TOTPUnbind, TOTPUnbindReq, TOTPUnbindRsp
//   - TOTPVerify, TOTPVerifyReq, TOTPVerifyRsp
//
// Routes
//   - POST     /api/2fa/totp/bind
//   - POST     /api/2fa/totp/check
//   - POST     /api/2fa/totp/confirm
//   - GET      /api/2fa/totp/status
//   - POST     /api/2fa/totp/unbind
//   - POST     /api/2fa/totp/verify
func Register() {
	servicetwofa.Enabled = true
	model.Register[*modeltwofa.TOTPDevice]()

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
