/*
用户登录流程：
1. POST /api/login → 提交用户名和密码
2. 如果用户已启用 MFA，登录请求必须同时提交 totp_code 或 backup_code 其中一个
3. 登录成功

MFA管理流程：
1. POST /api/mfa/totp/check → 检查用户是否启用MFA
2. POST /api/mfa/totp/bind → 绑定设备
3. POST /api/mfa/totp/confirm → 确认绑定
4. POST /api/mfa/totp/verify → 已登录用户日常 TOTP 验证使用
5. POST /api/mfa/totp/unbind → 解绑设备
6. GET /api/mfa/totp/status → 查看状态和设备摘要

核心接口

绑定流程：
- POST /api/mfa/totp/bind - 初始化 TOTP 绑定
- POST /api/mfa/totp/confirm - 确认绑定 TOTP 设备

验证流程：
- POST /api/mfa/totp/verify - 已登录用户验证 TOTP 代码，不参与登录流程

检查接口：
- POST /api/mfa/totp/check - 检查用户是否启用 MFA

管理接口：
- GET /api/mfa/totp/status - 获取用户 MFA 状态和脱敏设备摘要
- POST /api/mfa/totp/unbind - 解绑 TOTP 设备

核心服务逻辑

A. TOTP 绑定服务
- 生成随机密钥
- 创建 QR 码 URL
- 创建待确认绑定挑战
- 返回挑战 ID、二维码和绑定信息

B. TOTP 确认服务
- 读取并校验待确认绑定挑战
- 验证用户输入的 TOTP 代码
- 生成一次性恢复码
- 将恢复码哈希后保存
- 保存设备信息到数据库
- 激活 MFA 功能

C. TOTP 验证服务
- 验证已登录用户提交的 TOTP 代码
- 更新设备使用时间
- 返回验证结果

D. TOTP 解绑服务
- 验证用户身份和权限
- 通过密码、TOTP 代码或恢复码三选一完成 fresh auth
- 查找并解绑指定设备
- 统计剩余活跃设备数量
- 返回解绑结果

E. TOTP 检查服务
- 验证用户身份（用户名和密码）
- 查询用户的活跃 TOTP 设备
- 返回是否启用 MFA 的状态

F. TOTP 状态服务
- 查询用户的 MFA 状态
- 返回设备列表信息
*/

package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
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
//   - POST     /api/mfa/totp/bind
//   - POST     /api/mfa/totp/check
//   - POST     /api/mfa/totp/confirm
//   - GET      /api/mfa/totp/status
//   - POST     /api/mfa/totp/unbind
//   - POST     /api/mfa/totp/verify
func Register() {
	servicemfa.Enabled = true
	// The built-in module wires MFA to the framework IAM account store. Projects
	// using copied MFA source should install their own AccountAuthenticator from
	// project-owned code instead of editing service/mfa.
	servicemfa.SetAccountAuthenticator(iamAccountAuthenticator{})
	model.Register[*modelmfa.TOTPDevice]()

	module.Use[
		*TOTPBind,
		*TOTPBind,
		*TOTPBindRsp](
		&TOTPBindModule{},
		module.CRUD(consts.PHASE_CREATE),
	)

	module.Use[
		*TOTPCheck,
		*TOTPCheckReq,
		*TOTPCheckRsp](
		&TOTPCheckModule{},
		module.CRUD(consts.PHASE_CREATE),
	)

	module.Use[
		*TOTPConfirm,
		*TOTPConfirmReq,
		*TOTPConfirmRsp](
		&TOTPConfirmModule{},
		module.CRUD(consts.PHASE_CREATE),
	)

	module.Use[
		*TOTPStatus,
		*TOTPStatus,
		*TOTPStatusRsp](
		&TOTPStatusModule{},
		module.CRUD(consts.PHASE_LIST),
	)

	module.Use[
		*TOTPUnbind,
		*TOTPUnbindReq,
		*TOTPUnbindRsp](
		&TOTPUnbindModule{},
		module.CRUD(consts.PHASE_CREATE),
	)

	module.Use[
		*TOTPVerify,
		*TOTPVerifyReq,
		*TOTPVerifyRsp](
		&TOTPVerifyModule{},
		module.CRUD(consts.PHASE_CREATE),
	)
}
