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
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/module/logmgmt"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

var (
	token        = "-"
	port         = testutil.SetupRandomServerPort()
	rootUsername = "root"
	rootPassword = "12345678"

	signupAPI       = testutil.URL(port, "/api/signup")
	loginAPI        = testutil.URL(port, "/api/login")
	logoutAPI       = testutil.URL(port, "/api/logout")
	loginlogAPI     = testutil.URL(port, "/api/log/loginlog")
	operationlogAPI = testutil.URL(port, "/api/log/operationlog")
	roleAPI         = testutil.URL(port, "/api/authz/roles")
)

const (
	logmgmtTestReaderRole = "logmgmt_test_reader"
	logmgmtTestAdminRole  = "logmgmt_test_admin"
)

type ListResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_USERNAME, "test_module")
	os.Setenv(config.MYSQL_PASSWORD, "test_module")
	os.Setenv(config.MYSQL_DATABASE, "test_module")
	os.Setenv(config.REDIS_ENABLE, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLE, "true")
	os.Setenv(config.AUDIT_ASYNC_WRITE, "false")

	iam.Register(iam.Config{
		DefaultUsers: []*iam.DefaultUser{
			{
				Username: rootUsername,
				Password: rootPassword,
				ID:       "root",
			},
		},
	})
	authz.Register()
	logmgmt.Register()

	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	testutil.MustWaitForServer(port)
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
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.LoginLog]) {
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
		testutil.TestResp(t, resp, func(t *testing.T, rsp *iam.LogoutRsp) {
			t.Helper()
		})

		sessionID = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: username,
			Password: password,
		})

		cli := newLoginLogClient(t, sessionID)
		items := make([]*logmgmt.LoginLog, 0)
		total := new(int64)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.LoginLog]) {
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
	roleCode := logmgmtTestUsername("logmgmt_test_role")
	roleID := util.HashID(roleCode)
	clearOperationLogs(t)
	t.Cleanup(func() {
		clearOperationLogs(t)
	})

	t.Run("before_operation", func(t *testing.T) {
		cli := newOperationLogClient(t, sessionID, client.WithQuery("record_id", roleID))
		items := make([]*logmgmt.OperationLog, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.OperationLog]) {
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
		Code: roleCode,
	}
	resp, err := cli.Create(createReq)
	require.NoError(t, err)
	testutil.TestResp(t, resp, func(t *testing.T, rsp *authz.Role) {
		t.Helper()
		require.NotNil(t, rsp)
		require.Equal(t, createReq.Code, rsp.Code)
	})

	time.Sleep(1 * time.Second)
	t.Run("after_operation", func(t *testing.T) {
		cli := newOperationLogClient(t, sessionID, client.WithQuery("record_id", roleID))
		items := make([]*logmgmt.OperationLog, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.OperationLog]) {
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
	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
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

func clearOperationLogs(t *testing.T) {
	t.Helper()

	logs := make([]*logmgmt.OperationLog, 0)
	require.NoError(t, database.Database[*logmgmt.OperationLog](context.Background()).WithLimit(-1).List(&logs))
	if len(logs) == 0 {
		return
	}
	require.NoError(t, database.Database[*logmgmt.OperationLog](context.Background()).WithPurge().Delete(logs...))
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

	testutil.TestResp(t, apiResp, func(t *testing.T, rsp iam.LoginRsp) {
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
	require.NoError(t, perm.AssignRole(ctx, rbac.DefaultTenant, userID, logmgmtTestReaderRole))
	require.NoError(t, perm.GrantPermission(ctx, rbac.DefaultTenant, logmgmtTestReaderRole, "/api/log/loginlog", http.MethodGet))
	require.NoError(t, perm.GrantPermission(ctx, rbac.DefaultTenant, logmgmtTestReaderRole, "/api/log/operationlog", http.MethodGet))
	require.NoError(t, perm.GrantPermission(ctx, rbac.DefaultTenant, logmgmtTestReaderRole, "/api/logout", http.MethodPost))

	require.NoError(t, perm.AssignRole(ctx, rbac.DefaultTenant, "root", logmgmtTestAdminRole))
	require.NoError(t, perm.GrantPermission(ctx, rbac.DefaultTenant, logmgmtTestAdminRole, "/api/authz/roles", http.MethodPost))

	allowed, err := perm.Authorize(ctx, rbac.DefaultTenant, userID, "/api/log/loginlog", http.MethodGet)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = perm.Authorize(ctx, rbac.DefaultTenant, userID, "/api/log/operationlog", http.MethodGet)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = perm.Authorize(ctx, rbac.DefaultTenant, userID, "/api/logout", http.MethodPost)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = perm.Authorize(ctx, rbac.DefaultTenant, "root", "/api/authz/roles", http.MethodPost)
	require.NoError(t, err)
	require.True(t, allowed)

	t.Cleanup(func() {
		require.NoError(t, perm.UnassignRole(context.Background(), rbac.DefaultTenant, userID, logmgmtTestReaderRole))
		require.NoError(t, perm.UnassignRole(context.Background(), rbac.DefaultTenant, "root", logmgmtTestAdminRole))
		require.NoError(t, perm.RevokeRolePermissions(context.Background(), rbac.DefaultTenant, logmgmtTestReaderRole))
		require.NoError(t, perm.RevokeRolePermissions(context.Background(), rbac.DefaultTenant, logmgmtTestAdminRole))
	})
}
