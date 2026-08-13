package logmgmt_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
)

// TestCleanup pins the retention job contract: expired rows are removed batch
// by batch and the run reports success, driven by the configured retention.
func TestCleanup(t *testing.T) {
	// Produce login-log rows through real logins.
	username := logmgmtTestUsername("cleanup_user")
	password := "12345678"
	userID := signupLogmgmtTestUser(t, username, password)
	_ = loginSessionIDFromCookie(t, iam.LoginReq{Username: username, Password: password})

	rows := make([]*modellogmgmt.LoginLog, 0)
	require.NoError(t, database.Database[*modellogmgmt.LoginLog](context.Background()).
		WithQuery(&modellogmgmt.LoginLog{UserID: userID}).
		List(&rows))
	require.NotEmpty(t, rows, "the login above must have produced log rows")

	// A negative retention makes every existing row expired from the job's
	// point of view without having to forge created_at timestamps.
	originalRetention := config.App.Logmgmt.Retention
	t.Cleanup(func() { config.App.Logmgmt.Retention = originalRetention })
	config.App.Logmgmt.Retention = -time.Hour

	require.NoError(t, servicelogmgmt.Cleanup())

	remaining := make([]*modellogmgmt.LoginLog, 0)
	require.NoError(t, database.Database[*modellogmgmt.LoginLog](context.Background()).
		WithQuery(&modellogmgmt.LoginLog{UserID: userID}).
		List(&remaining))
	require.Empty(t, remaining, "expired rows must be physically removed")
}
