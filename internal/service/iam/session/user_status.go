package serviceiamsession

import (
	"net/http"

	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
)

// ensureSessionUserActive verifies that the authenticated user can keep using an existing session.
func ensureSessionUserActive(user *modeliamuser.User) error {
	switch user.Status {
	case modeliamuser.UserStatusInactive:
		return service.NewError(http.StatusForbidden, "account disabled")
	case modeliamuser.UserStatusLocked:
		return service.NewError(http.StatusForbidden, "account locked")
	default:
		return nil
	}
}
