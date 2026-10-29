package module

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// auditLogin writes an audit row for every login lifecycle moment the iam
// module reports, a login that succeeded or failed and a logout. An
// observer runs on the request's own context, so the row is written the
// way a service writes one; it answers nothing, so a write that fails is
// logged and the login goes on.
func auditLogin(ctx *gst.ServiceContext, event authn.LoginEvent) {
	audit := &model.Audit{Action: "login_" + string(event.Kind), Actor: event.Username}
	if err := database.Database[*model.Audit](ctx).Create(audit); err != nil {
		logger.App.Errorz("audit login", zap.String("username", event.Username), zap.Error(err))
	}
}
