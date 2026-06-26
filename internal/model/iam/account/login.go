package modeliamaccount

type LoginReq struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	TOTPCode   string `json:"totp_code,omitempty"`   // Optional TOTP code
	BackupCode string `json:"backup_code,omitempty"` // Optional backup code
}
