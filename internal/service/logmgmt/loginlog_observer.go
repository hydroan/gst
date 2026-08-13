package servicelogmgmt

import (
	"fmt"

	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// The login-log service wires itself into the IAM login flow at package
// initialization: importing this package — through module/logmgmt on the add
// path or the copied service package on the copy path — is what starts
// recording login lifecycle events. No separate switch exists.
func init() {
	authn.AddLoginObserver(recordLoginEvent)
}

// recordLoginEvent persists one login lifecycle event as a LoginLog row.
//
// Observers never block or fail the login itself, so a failed insert is only
// logged. The user-agent columns keep the historical "<name> <version>" and
// "<platform> <os>" renderings so rows stay comparable across versions.
func recordLoginEvent(ctx *types.ServiceContext, event authn.LoginEvent) {
	entry := &modellogmgmt.LoginLog{
		UserID:   event.UserID,
		Username: event.Username,
		ClientIP: event.ClientIP,
		Status:   loginLogStatus(event.Kind),
		Source:   event.UserAgent,
		Platform: fmt.Sprintf("%s %s", event.Platform, event.OS),
		Engine:   fmt.Sprintf("%s %s", event.EngineName, event.EngineVersion),
		Browser:  fmt.Sprintf("%s %s", event.BrowserName, event.BrowserVersion),
	}
	if err := database.Database[*modellogmgmt.LoginLog](ctx).Create(entry); err != nil {
		// logger.App stays nil until logger initialization; the observer must
		// not require a configured logger to stay safe.
		if logger.App != nil {
			logger.App.Warnz("failed to write login log",
				zap.String("username", event.Username),
				zap.String("status", string(entry.Status)),
				zap.Error(err))
		}
	}
}

// loginLogStatus maps an authn event kind onto the persisted status column.
func loginLogStatus(kind authn.LoginEventKind) modellogmgmt.LoginStatus {
	switch kind {
	case authn.LoginEventSucceeded:
		return modellogmgmt.LoginStatusSuccess
	case authn.LoginEventFailed:
		return modellogmgmt.LoginStatusFailure
	case authn.LoginEventLoggedOut:
		return modellogmgmt.LoginStatusLogout
	default:
		return modellogmgmt.LoginStatus(kind)
	}
}
