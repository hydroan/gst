package logmgmt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/module/logmgmt"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

var (
	rootUsername = "root"
	rootPassword = "12345678"

	baseURL = testutil.BaseURL()
)

const (
	signupPath       = "/api/signup"
	loginPath        = "/api/login"
	logoutPath       = "/api/logout"
	loginlogPath     = "/api/log/loginlog"
	operationlogPath = "/api/log/operationlog"
	rolePath         = "/api/authz/roles"
)

const (
	logmgmtTestReaderRole = "logmgmt_test_reader"
	logmgmtTestAdminRole  = "logmgmt_test_admin"
)

func TestMain(m *testing.M) {
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLED, "true")
	os.Setenv(config.AUDIT_ASYNC_WRITE, "false")

	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Register: func() {
			iam.Register()
			authz.Register()
			logmgmt.Register()
		},
		Seed: seedRootAccount,
	})
}

// seedRootAccount creates the root user and password credential the tests
// authenticate with. Baseline accounts are application data, so the test
// creates them explicitly through the standard database chain.
func seedRootAccount() {
	ctx := context.Background()

	user := &modeliamuser.User{Username: rootUsername, Status: modeliamuser.UserStatusActive}
	user.ID = "root"
	if err := database.Database[*modeliamuser.User](ctx).Create(user); err != nil {
		panic(err)
	}

	credential, err := serviceiamaccount.NewPasswordCredential(ctx, user.ID, rootPassword, false)
	if err != nil {
		panic(err)
	}
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); err != nil {
		panic(err)
	}
}

func TestLoginLogList(t *testing.T) {
	t.Skip("IAM login log integration is temporarily disabled.")

	username := logmgmtTestUsername("loginlog_user")
	password := "12345678"
	userID := signupLogmgmtTestUser(t, username, password)
	grantLogmgmtTestPermissions(t, userID)

	sessionID := loginSessionIDFromCookie(t, iam.LoginReq{
		Username: username,
		Password: password,
	})

	t.Run("after_login", func(t *testing.T) {
		cli := logmgmtSessionClient(t, sessionID)

		list, err := client.Get[client.ListResult[*logmgmt.LoginLog]](cli, loginlogPath)
		require.NoError(t, err)

		require.Len(t, list.Items, 1)
		l := list.Items[0]
		require.Equal(t, userID, l.UserID)
		require.Equal(t, username, l.Username)
		require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l.Status))
	})

	t.Run("after_logout_and_login_again", func(t *testing.T) {
		logoutCli := logmgmtSessionClient(t, sessionID)
		_, err := client.Post[iam.LogoutRsp](logoutCli, logoutPath, nil)
		require.NoError(t, err)

		sessionID = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: username,
			Password: password,
		})

		cli := logmgmtSessionClient(t, sessionID)

		list, err := client.Get[client.ListResult[*logmgmt.LoginLog]](cli, loginlogPath)
		require.NoError(t, err)

		require.Len(t, list.Items, 3)
		l1, l2, l3 := list.Items[0], list.Items[1], list.Items[2]

		require.Equal(t, userID, l1.UserID)
		require.Equal(t, username, l1.Username)
		require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l1.Status))

		require.Equal(t, userID, l2.UserID)
		require.Equal(t, username, l2.Username)
		require.Equal(t, modellogmgmt.LoginStatusLogout, string(l2.Status))

		require.Equal(t, userID, l3.UserID)
		require.Equal(t, username, l3.Username)
		require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l3.Status))
	})
}

func TestOperationLogList(t *testing.T) {
	username := logmgmtTestUsername("operationlog_user")
	password := "12345678"
	userID := signupLogmgmtTestUser(t, username, password)
	grantLogmgmtTestPermissions(t, userID)
	sessionID := loginSessionIDFromCookie(t, iam.LoginReq{
		Username: username,
		Password: password,
	})
	roleName := logmgmtTestUsername("logmgmt_test_role")
	roleID := util.HashID(roleName)
	t.Run("before_operation", func(t *testing.T) {
		cli := logmgmtSessionClient(t, sessionID)

		list, err := client.Get[client.ListResult[*logmgmt.OperationLog]](cli, operationlogPath,
			client.WithQuery("record_id", roleID))
		require.NoError(t, err)
		require.Empty(t, list.Items)
	})

	adminSessionID := loginSessionIDFromCookie(t, iam.LoginReq{
		Username: rootUsername,
		Password: rootPassword,
	})
	cli := logmgmtSessionClient(t, adminSessionID)
	createReq := &authz.Role{
		Base: model.Base{ID: roleID},
		Name: roleName,
	}
	created, err := client.Post[authz.Role](cli, rolePath, createReq)
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, createReq.Name, created.Name)

	time.Sleep(1 * time.Second)
	t.Run("after_operation", func(t *testing.T) {
		cli := logmgmtSessionClient(t, sessionID)

		list, err := client.Get[client.ListResult[*logmgmt.OperationLog]](cli, operationlogPath,
			client.WithQuery("record_id", roleID))
		require.NoError(t, err)

		require.Len(t, list.Items, 1)
		l := list.Items[0]
		require.NotNil(t, l)
		require.Equal(t, rootUsername, l.User)
		require.Equal(t, consts.OP_CREATE, l.OP)
		require.Equal(t, "roles", l.Table)
		require.Equal(t, "Role", l.Model)
		require.Equal(t, "/api/authz/roles", l.URI)
	})
}

func signupLogmgmtTestUser(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(baseURL)
	require.NoError(t, err)
	rsp, err := client.Post[iam.SignupRsp](cli, signupPath, iam.SignupReq{
		Username:   username,
		Password:   password,
		RePassword: password,
	})
	require.NoError(t, err)
	require.Equal(t, username, rsp.Username)
	require.NotEmpty(t, rsp.UserID)
	require.NotEmpty(t, rsp.Message)
	return rsp.UserID
}

// logmgmtSessionClient returns a client that presents the given session id.
func logmgmtSessionClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func logmgmtTestUsername(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	apiResp, err := cli.Do(http.MethodPost, loginPath, reqPayload)
	require.NoError(t, err)

	rsp := testutil.DecodeResp[iam.LoginRsp](t, apiResp)
	require.False(t, rsp.ServerTime.IsZero())
	require.False(t, rsp.Session.ExpiresAt.IsZero())

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	cookie := apiResp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	require.NotEmpty(t, cookie.Value)
	require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
	return cookie.Value
}

func grantLogmgmtTestPermissions(t *testing.T, userID string) {
	t.Helper()

	perm := rbac.RBAC()
	ctx := context.Background()
	require.NoError(t, perm.AssignRole(ctx, tenant.Default, userID, logmgmtTestReaderRole))
	require.NoError(t, perm.SetRolePermissions(ctx, tenant.Default, logmgmtTestReaderRole, []types.Permission{
		{Object: "/api/log/loginlog", Action: http.MethodGet},
		{Object: "/api/log/operationlog", Action: http.MethodGet},
		{Object: "/api/logout", Action: http.MethodPost},
	}))

	require.NoError(t, perm.AssignRole(ctx, tenant.Default, "root", logmgmtTestAdminRole))
	require.NoError(t, perm.SetRolePermissions(ctx, tenant.Default, logmgmtTestAdminRole, []types.Permission{
		{Object: "/api/authz/roles", Action: http.MethodPost},
	}))

	decision, err := perm.Authorize(ctx, tenant.Default, userID, "/api/log/loginlog", http.MethodGet)
	allowed := decision.Allowed
	require.NoError(t, err)
	require.True(t, allowed)
	decision, err = perm.Authorize(ctx, tenant.Default, userID, "/api/log/operationlog", http.MethodGet)
	allowed = decision.Allowed
	require.NoError(t, err)
	require.True(t, allowed)
	decision, err = perm.Authorize(ctx, tenant.Default, userID, "/api/logout", http.MethodPost)
	allowed = decision.Allowed
	require.NoError(t, err)
	require.True(t, allowed)
	decision, err = perm.Authorize(ctx, tenant.Default, "root", "/api/authz/roles", http.MethodPost)
	allowed = decision.Allowed
	require.NoError(t, err)
	require.True(t, allowed)
}
