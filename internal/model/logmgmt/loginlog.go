package modellogmgmt

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type LoginStatus string

const (
	LoginStatusSuccess = "success"
	LoginStatusFailure = "failure"
	LoginStatusLogout  = "logout"
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

func (LoginLog) Design() {
	Migrate()
	List(func() {
		Enabled(true)
	})
	Get(func() {
		Enabled(true)
	})
}
