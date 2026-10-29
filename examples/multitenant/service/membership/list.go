package membership

import (
	"net/http"

	"multitenant/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authz/rbac"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Lister answers GET /api/memberships, a custom List: the tenants the
// caller holds a role in, read from the role bindings the authz module
// keeps, and the tenant the request acts in.
type Lister struct {
	service.Base[*model.Membership, *gstmodel.Empty, *model.MembershipListRsp]
}

// List asks the authorization for the caller's tenants, the third reading
// of the relation role bindings record, beside the roles a subject holds in
// a tenant and the subjects a tenant holds. Current is the tenant the
// request acts in: the one the login named, or the default tenant.
func (m *Lister) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.MembershipListRsp, error) {
	tenants, err := rbac.RBAC().TenantsForSubject(ctx, ctx.UserID())
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to list the memberships", err)
	}
	if tenants == nil {
		tenants = make([]string, 0)
	}
	return &model.MembershipListRsp{Tenants: tenants, Current: ctx.TenantID()}, nil
}
