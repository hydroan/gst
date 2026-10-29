package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Membership answers which tenants the caller may act in: an action model
// with no table, GET /api/memberships, served by service/membership from
// the role bindings the authz module keeps. A client asks it before
// choosing the tenant to log in to, so the seed grants it to every
// authenticated subject.
type Membership struct {
	model.Empty
}

func (Membership) Design() {
	Route("memberships", func() {
		List(func() {
			Service()
			Result[*MembershipListRsp]()
		})
	})
}

// MembershipListRsp is the tenants the caller holds a role in, and the one
// the session acts in.
type MembershipListRsp struct {
	Tenants []string `json:"tenants"`
	Current string   `json:"current"`
}
