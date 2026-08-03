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

	signupAPI       = testutil.URL("/api/signup")
	loginAPI        = testutil.URL("/api/login")
	logoutAPI       = testutil.URL("/api/logout")
	loginlogAPI     = testutil.URL("/api/log/loginlog")
	operationlogAPI = testutil.URL("/api/log/operationlog")
	roleAPI         = testutil.URL("/api/authz/roles")
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
		cli := newLoginLogClient(t, sessionID)
		items := make([]*logmgmt.LoginLog, 0)
		total := new(int)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.RequireResp(t, resp, func(t *testing.T, rsp testutil.ListResponse[*logmgmt.LoginLog]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			l := rsp.Items[0]
			require.Equal(t, userID, l.UserID)
			require.Equal(t, username, l.Username)
			require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l.Status))
		})
	})

	t.Run("after_logout_and_login_again", func(t *testing.T) {
		logoutCli, err := client.New(logoutAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)
		resp, err := logoutCli.Create(nil)
		require.NoError(t, err)
		testutil.RequireResp(t, resp, func(t *testing.T, rsp *iam.LogoutRsp) {
			t.Helper()
		})

		sessionID = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: username,
			Password: password,
		})

		cli := newLoginLogClient(t, sessionID)
		items := make([]*logmgmt.LoginLog, 0)
		total := new(int)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)

		testutil.RequireResp(t, resp, func(t *testing.T, rsp testutil.ListResponse[*logmgmt.LoginLog]) {
			t.Helper()
			require.Len(t, rsp.Items, 3)
			l1, l2, l3 := rsp.Items[0], rsp.Items[1], rsp.Items[2]

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
		cli := newOperationLogClient(t, sessionID, client.WithQuery("record_id", roleID))
		items := make([]*logmgmt.OperationLog, 0)
		total := new(int)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.RequireResp(t, resp, func(t *testing.T, rsp testutil.ListResponse[*logmgmt.OperationLog]) {
			t.Helper()
			require.Empty(t, rsp.Items)
		})
	})

	adminSessionID := loginSessionIDFromCookie(t, iam.LoginReq{
		Username: rootUsername,
		Password: rootPassword,
	})
	cli, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: adminSessionID,
	}))
	require.NoError(t, err)
	createReq := &authz.Role{
		Base: model.Base{ID: roleID},
		Name: roleName,
	}
	resp, err := cli.Create(createReq)
	require.NoError(t, err)
	testutil.RequireResp(t, resp, func(t *testing.T, rsp *authz.Role) {
		t.Helper()
		require.NotNil(t, rsp)
		require.Equal(t, createReq.Name, rsp.Name)
	})

	time.Sleep(1 * time.Second)
	t.Run("after_operation", func(t *testing.T) {
		cli := newOperationLogClient(t, sessionID, client.WithQuery("record_id", roleID))
		items := make([]*logmgmt.OperationLog, 0)
		total := new(int)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.RequireResp(t, resp, func(t *testing.T, rsp testutil.ListResponse[*logmgmt.OperationLog]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			l := rsp.Items[0]
			require.NotNil(t, l)
			require.Equal(t, rootUsername, l.User)
			require.Equal(t, consts.OP_CREATE, l.OP)
			require.Equal(t, "roles", l.Table)
			require.Equal(t, "Role", l.Model)
			require.Equal(t, "/api/authz/roles", l.URI)
		})
	})
}

func signupLogmgmtTestUser(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(signupAPI)
	require.NoError(t, err)
	resp, err := cli.Create(iam.SignupReq{
		Username:   username,
		Password:   password,
		RePassword: password,
	})
	require.NoError(t, err)

	var userID string
	testutil.RequireResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
		t.Helper()
		require.Equal(t, username, rsp.Username)
		require.NotEmpty(t, rsp.UserID)
		require.NotEmpty(t, rsp.Message)
		userID = rsp.UserID
	})
	return userID
}

func newLoginLogClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(loginlogAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func newOperationLogClient(t *testing.T, sessionID string, opts ...client.Option) *client.Client {
	t.Helper()

	options := []client.Option{client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})}
	options = append(options, opts...)
	cli, err := client.New(operationlogAPI, options...)
	require.NoError(t, err)
	return cli
}

func logmgmtTestUsername(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	cli, err := client.New(loginAPI)
	require.NoError(t, err)

	apiResp, err := cli.Create(reqPayload)
	require.NoError(t, err)

	testutil.RequireResp(t, apiResp, func(t *testing.T, rsp iam.LoginRsp) {
		t.Helper()
		require.False(t, rsp.ServerTime.IsZero())
		require.False(t, rsp.Session.ExpiresAt.IsZero())
	})

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	for _, cookie := range apiResp.Cookies {
		if cookie.Name != "session_id" {
			continue
		}
		require.NotEmpty(t, cookie.Value)
		require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
		return cookie.Value
	}

	require.FailNow(t, "session cookie not found")
	return ""
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
