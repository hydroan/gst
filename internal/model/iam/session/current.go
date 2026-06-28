package modeliamsession

// CurrentGetReq is the request payload for getting the current session.
type CurrentGetReq struct{}

// CurrentGetRsp returns the current session together with the latest principal snapshot.
type CurrentGetRsp struct {
	Session   SessionView      `json:"session"`
	Principal CurrentPrincipal `json:"principal"`
}

// CurrentDeleteReq is the request payload for deleting the current session.
type CurrentDeleteReq struct{}

// CurrentDeleteRsp is the response payload for deleting the current session.
type CurrentDeleteRsp struct{}

// CurrentPrincipal describes the authenticated principal bound to the current session.
type CurrentPrincipal struct {
	UserID             string  `json:"user_id"`
	Username           string  `json:"username"`
	Email              string  `json:"email"`
	FirstName          *string `json:"first_name,omitempty"`
	LastName           *string `json:"last_name,omitempty"`
	Status             string  `json:"status"`
	MustChangePassword bool    `json:"must_change_password"`
}
