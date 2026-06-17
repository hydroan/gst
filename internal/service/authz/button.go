package serviceauthz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
)

type ButtonService struct {
	service.Base[*modelauthz.Button, *modelauthz.Button, *modelauthz.Button]
}
