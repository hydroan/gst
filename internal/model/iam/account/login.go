package modeliamaccount

import (
	. "github.com/hydroan/gst/dsl"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
)

type Login struct {
	model.Empty
}
type LoginReq struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	TenantID   string `json:"tenant_id,omitempty" query:"tenant_id"`
	TOTPCode   string `json:"totp_code,omitempty"`   // Optional TOTP code
	BackupCode string `json:"backup_code,omitempty"` // Optional backup code
}

// LoginRsp returns the authenticated session created by a successful login.
type LoginRsp = modeliamsession.AuthenticatedSessionRsp

func (Login) Design() {
	Route("/login", func() {
		Create(func() {
			Service()
			Flatten()
			Public()
			Filename("login.go")
			Payload[*LoginReq]()
			Result[*LoginRsp]()
		})
	})
}
