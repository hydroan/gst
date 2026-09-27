package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminTOTP is the administrative view over one target account's TOTP
// enrollment, addressed by the target user id in the route path.
type AdminTOTP struct {
	model.Empty
}

// AdminTOTPResetRsp reports the outcome of an administrative TOTP reset.
type AdminTOTPResetRsp struct {
	RemovedDeviceCount int `json:"removed_device_count"`
}

func (AdminTOTP) Design() {
	Route("mfa/admin/users/:id/totp", func() {
		Get(func() {
			Flatten()
			Exact()
			Service("admin_totp_status")
			Result[*TOTPStatusRsp]()
		})
		Delete(func() {
			Flatten()
			Exact()
			Service("admin_totp_reset")
			Result[*AdminTOTPResetRsp]()
		})
	})
}
