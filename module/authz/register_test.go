package authz_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/helper"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/response"
	"github.com/stretchr/testify/require"
)

var (
	token        = "-"
	port         = 8000
	rootUsername = "root"
	rootPassword = "12345678"

	signupAPI = fmt.Sprintf("http://localhost:%d/api/signup", port)
	loginAPI  = fmt.Sprintf("http://localhost:%d/api/login", port)

	apiAPI        = fmt.Sprintf("http://localhost:%d/api/apis", port)
	menuAPI       = fmt.Sprintf("http://localhost:%d/api/menus", port)
	permissionAPI = fmt.Sprintf("http://localhost:%d/api/authz/permissions", port)
	roleAPI       = fmt.Sprintf("http://localhost:%d/api/authz/roles", port)
	userRoleAPI   = fmt.Sprintf("http://localhost:%d/api/authz/user-roles", port)
)

type ListResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "true")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(port))
	os.Setenv(config.REDIS_ENABLE, "true")
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLE, "true")
	os.Setenv(config.AUDIT_ASYNC_WRITE, "false")

	iam.Register(iam.Config{
		DefaultUsers: []*iam.User{
			{
				Base:     model.Base{ID: "root"},
				Type:     "admin",
				Username: rootUsername,
				Password: rootPassword,
			},
		},
	})
	authz.Register()
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	for {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			l.Close()
			time.Sleep(1 * time.Second)
			continue
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			break
		}
		panic(err)

	}
}

func TestAuthz(t *testing.T) {
	username := "user01"
	password := "12345678"
	userID := ""

	t.Run("signup", func(t *testing.T) {
		cli, err := client.New(signupAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.SignupReq{
			Username:   username,
			Password:   password,
			RePassword: password,
		})
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
			t.Helper(
			// #modeliam.SignupRsp {
			//   +UserID   => "019cbca0-19d4-7971-8be5-65b148027a27" #string
			//   +Username => "user01" #string
			//   +Message  => "User created successfully" #string
			// }
			)

			require.Equal(t, rsp.Username, username)
			require.NotEmpty(t, rsp.UserID)
			require.NotEmpty(t, rsp.Message)
			userID = rsp.UserID
		})
	})

	// Authz management endpoints require the built-in admin role. The user created
	// in signup only covers the authentication flow, so the tests keep a dedicated
	// root session for listing and mutating authz resources.
	var adminSessionID string
	t.Run("login", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
			t.Helper(
			// #*modeliam.LoginRsp {
			//   +SessionID => "019cbca0-1a0b-7a12-8264-4c0525076cd6" #string
			// }
			)

			require.NotEmpty(t, rsp.SessionID)
		})
	})
	t.Run("login_root", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: rootUsername,
			Password: rootPassword,
		})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.SessionID)
			adminSessionID = rsp.SessionID
		})
	})
	t.Run("api", func(t *testing.T) {
		cli, err := client.New(apiAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp []string) {
			t.Helper(
			// #[]string [
			//   0 => "/api/iam/session/heartbeat" #string
			//   1 => "/api/signup" #string
			//   2 => "/api/iam/groups" #string
			//   3 => "/api/menus/{id}" #string
			//   4 => "/api/authz/user-roles" #string
			//   5 => "/api/authz/user-roles/{id}" #string
			//   6 => "/api/login" #string
			//   7 => "/api/iam/session/current" #string
			//   8 => "/api/iam/groups/{id}" #string
			//   9 => "/api/iam/users/{id}" #string
			//   10 => "/api/online-users" #string
			//   11 => "/api/authz/roles/{id}" #string
			//   12 => "/api/apis" #string
			//   13 => "/api/iam/change-password" #string
			//   14 => "/api/logout" #string
			//   15 => "/api/authz/permissions" #string
			//   16 => "/api/iam/users" #string
			//   17 => "/api/buttons" #string
			//   18 => "/api/buttons/{id}" #string
			//   19 => "/api/authz/permissions/{id}" #string
			//   20 => "/api/menus" #string
			//   21 => "/api/authz/roles" #string
			// ]
			)

			require.NotEmpty(t, rsp, "apis list should not be empty")
		})
	})

	t.Run("permission", func(t *testing.T) {
		cli, err := client.New(permissionAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		items := make([]*authz.Permission, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)
		helper.TestResp[ListResponse[*authz.Permission]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Permission]) {
			t.Helper()
			require.NotNil(t, rsp.Items)
			require.GreaterOrEqual(t, rsp.Total, int64(0))
		})
	})

	t.Run("menu", func(t *testing.T) {
		cli, err := client.New(menuAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var menuID string
		var resp *client.Resp
		var cliExpand *client.Client

		t.Run("list", func(t *testing.T) {
			items := make([]*authz.Menu, 0)
			total := new(int64)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.Menu{
				ParentID: "root",
				Label:    "Test Menu",
				Path:     "/test",
				API:      []string{"/api/test"},
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, createReq.Label, rsp.Label)
				require.Equal(t, createReq.Path, rsp.Path)
				require.Equal(t, createReq.ParentID, rsp.ParentID)
				menuID = rsp.ID
			})
		})

		t.Run("get", func(t *testing.T) {
			got := new(authz.Menu)
			resp, err = cli.Get(menuID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, "Test Menu", rsp.Label)
				require.Equal(t, "/test", rsp.Path)
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Menu{
				ParentID: "root",
				Label:    "Test Menu Updated",
				Path:     "/test-updated",
				API:      []string{"/api/test", "/api/test-updated"},
			}
			resp, err = cli.Update(menuID, updateReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, updateReq.Label, rsp.Label)
				require.Equal(t, updateReq.Path, rsp.Path)
			})
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.Menu{Label: "Test Menu Patched"}
			resp, err = cli.Patch(menuID, patchReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, patchReq.Label, rsp.Label)
				require.Equal(t, "/test-updated", rsp.Path)
			})
		})

		t.Run("list_expand", func(t *testing.T) {
			cliExpand, err = client.New(menuAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}), client.WithQueryExpand("Children,Parent", 1))
			require.NoError(t, err)
			items := make([]*authz.Menu, 0)
			total := new(int64)
			resp, err = cliExpand.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(menuID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, response.CodeSuccess.Code(), resp.Code, "delete should return success")
		})
	})

	t.Run("role", func(t *testing.T) {
		cli, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var roleID string
		var resp *client.Resp

		t.Run("list", func(t *testing.T) {
			items := make([]*authz.Role, 0)
			total := new(int64)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.Role]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Role]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.Role{
				Name: "Test Role",
				Code: "test_role",
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, createReq.Name, rsp.Name)
				require.Equal(t, createReq.Code, rsp.Code)
				roleID = rsp.ID
			})
		})

		t.Run("get", func(t *testing.T) {
			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, "Test Role", rsp.Name)
				require.Equal(t, "test_role", rsp.Code)
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Role{
				Name: "Test Role Updated",
				Code: "test_role_updated",
			}
			resp, err = cli.Update(roleID, updateReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, updateReq.Name, rsp.Name)
				require.Equal(t, updateReq.Code, rsp.Code)
			})
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.Role{Name: "Test Role Patched"}
			resp, err = cli.Patch(roleID, patchReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, patchReq.Name, rsp.Name)
				require.Equal(t, "test_role_updated", rsp.Code)
			})
		})

		t.Run("list_expand", func(t *testing.T) {
			items := make([]*authz.Role, 0)
			total := new(int64)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.Role]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Role]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(roleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, response.CodeSuccess.Code(), resp.Code, "delete should return success")
		})
	})

	t.Run("user_role", func(t *testing.T) {
		cli, err := client.New(userRoleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var userRoleID string
		var roleID string
		var resp *client.Resp

		// Create a role for assigning to user (role from previous test was deleted).
		cliRole, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		resp, err = cliRole.Create(&authz.Role{Name: "UserRole Test Role", Code: "userrole_test_role"})
		require.NoError(t, err)
		helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			roleID = rsp.ID
		})

		t.Run("list", func(t *testing.T) {
			items := make([]*authz.UserRole, 0)
			total := new(int64)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.UserRole]](t, resp, func(t *testing.T, rsp ListResponse[*authz.UserRole]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.UserRole{
				UserID: userID,
				RoleID: roleID,
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			helper.TestResp[*authz.UserRole](t, resp, func(t *testing.T, rsp *authz.UserRole) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, userID, rsp.UserID)
				require.Equal(t, roleID, rsp.RoleID)
				require.NotEmpty(t, rsp.Username)
				require.NotEmpty(t, rsp.RoleCode)
				userRoleID = rsp.ID
			})
		})

		t.Run("get", func(t *testing.T) {
			got := new(authz.UserRole)
			resp, err = cli.Get(userRoleID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.UserRole](t, resp, func(t *testing.T, rsp *authz.UserRole) {
				t.Helper()
				require.Equal(t, userRoleID, rsp.ID)
				require.Equal(t, userID, rsp.UserID)
				require.Equal(t, roleID, rsp.RoleID)
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.UserRole{
				UserID: userID,
				RoleID: roleID,
			}
			resp, err = cli.Update(userRoleID, updateReq)
			require.NoError(t, err)
			helper.TestResp[*authz.UserRole](t, resp, func(t *testing.T, rsp *authz.UserRole) {
				t.Helper()
				require.Equal(t, userRoleID, rsp.ID)
				require.Equal(t, userID, rsp.UserID)
				require.Equal(t, roleID, rsp.RoleID)
			})
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.UserRole{UserID: userID}
			resp, err = cli.Patch(userRoleID, patchReq)
			require.NoError(t, err)
			helper.TestResp[*authz.UserRole](t, resp, func(t *testing.T, rsp *authz.UserRole) {
				t.Helper()
				require.Equal(t, userRoleID, rsp.ID)
				require.Equal(t, userID, rsp.UserID)
				require.Equal(t, roleID, rsp.RoleID)
			})
		})

		t.Run("list_expand", func(t *testing.T) {
			items := make([]*authz.UserRole, 0)
			total := new(int64)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.UserRole]](t, resp, func(t *testing.T, rsp ListResponse[*authz.UserRole]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, int64(0))
			})
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(userRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, response.CodeSuccess.Code(), resp.Code, "delete should return success")
		})
	})
}
