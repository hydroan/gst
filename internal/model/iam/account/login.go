package modeliamaccount

import modeliamsession "github.com/hydroan/gst/internal/model/iam/session"

type LoginReq struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	TOTPCode   string `json:"totp_code,omitempty"`   // Optional TOTP code
	BackupCode string `json:"backup_code,omitempty"` // Optional backup code
}

// LoginRsp returns the authenticated session created by a successful login.
type LoginRsp = modeliamsession.AuthenticatedSessionRsp
