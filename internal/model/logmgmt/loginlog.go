package modellogmgmt

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type LoginStatus string

const (
	LoginStatusSuccess LoginStatus = "success"
	LoginStatusFailure LoginStatus = "failure"
	LoginStatusLogout  LoginStatus = "logout"
)

type LoginLog struct {
	// User Info
	UserID   string      `json:"user_id,omitempty" query:"user_id"`
	Username string      `json:"username,omitempty" query:"username"`
	ClientIP string      `json:"client_ip,omitempty" query:"client_ip"`
	Status   LoginStatus `json:"status,omitempty" query:"status"`

	// User Agent info
	Source   string `json:"source" query:"source"`
	Platform string `json:"platform" query:"platform"`
	Engine   string `json:"engine" query:"engine"`
	Browser  string `json:"browser" query:"browser"`

	model.Base
}

// TableName pins the table name gorm would otherwise derive.
func (LoginLog) TableName() string { return "login_logs" }

// Purge makes every LoginLog deletion a hard delete: the retention cronjob
// removes expired rows to reclaim space, and a soft-deleted audit trail row
// would defeat that while pretending the log was trimmed.
func (LoginLog) Purge() bool { return true }

func (LoginLog) Design() {
	Migrate()
	// The route matches the add path registration so the copy path generates
	// the same endpoints instead of a diverging default prefix.
	Route("log/loginlog", func() {
		List(func() {
			Enabled(true)
		})
		Get(func() {
			Enabled(true)
		})
	})
}
