package modelauthz

import "github.com/hydroan/gst/model"

type User struct {
	Username string `json:"username,omitempty" schema:"username"`

	model.Base
}
