package authz_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/helper"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
)

const testSuccessCode = 0

var (
	token        = "-"
	port         = 8000
	rootUsername = "root"
	rootPassword = "12345678"

	signupAPI = fmt.Sprintf("http://localhost:%d/api/signup", port)
	loginAPI  = fmt.Sprintf("http://localhost:%d/api/login", port)

	routesAPI   = fmt.Sprintf("http://localhost:%d/api/routes", port)
	menuAPI     = fmt.Sprintf("http://localhost:%d/api/menus", port)
	roleAPI     = fmt.Sprintf("http://localhost:%d/api/authz/roles", port)
	userRoleAPI = fmt.Sprintf("http://localhost:%d/api/authz/user-roles", port)
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
	var userSessionID string
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
			userSessionID = rsp.SessionID
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
	t.Run("routes", func(t *testing.T) {
		cli, err := client.New(routesAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp authz.RoutesRsp) {
			t.Helper(
			// #modelauthz.RoutesRsp {
			//   +Items => []modelauthz.Route [
			//     0 => {
			//       +Path    => "/api/authz/roles" #string
			//       +Methods => ["GET", "POST"] #[]string
			//     }
			//   ]
			// }
			)

			require.NotEmpty(t, rsp.Items, "routes list should not be empty")
			requireRoute(t, rsp.Items, "/api/routes", []string{http.MethodGet})
			requireRoute(t, rsp.Items, "/api/authz/roles", []string{http.MethodGet, http.MethodPost})
			requireRoute(t, rsp.Items, "/api/authz/roles/{id}", []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete})
			requireRoute(t, rsp.Items, "/api/authz/user-roles", []string{http.MethodGet, http.MethodPost})
			requireRoute(t, rsp.Items, "/api/authz/user-roles/{id}", []string{http.MethodGet, http.MethodDelete})
			requireNoRoute(t, rsp.Items, "/api/authz/permissions")
			requireNoRoute(t, rsp.Items, "/api/authz/permissions/{id}")
			requireNoRoute(t, rsp.Items, "/api/authz/roles/:id")
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
				Routes: []authz.Route{
					{Path: "/api/routes", Methods: []string{http.MethodGet}},
				},
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, createReq.Label, rsp.Label)
				require.Equal(t, createReq.Path, rsp.Path)
				require.Equal(t, createReq.ParentID, rsp.ParentID)
				require.Equal(t, createReq.Routes, rsp.Routes)
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
				require.Equal(t, []authz.Route{{Path: "/api/routes", Methods: []string{http.MethodGet}}}, []authz.Route(rsp.Routes))
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Menu{
				ParentID: "root",
				Label:    "Test Menu Updated",
				Path:     "/test-updated",
				Routes: []authz.Route{
					{Path: "/api/routes", Methods: []string{http.MethodGet}},
					{Path: "/api/authz/roles", Methods: []string{http.MethodGet}},
				},
			}
			resp, err = cli.Update(menuID, updateReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, updateReq.Label, rsp.Label)
				require.Equal(t, updateReq.Path, rsp.Path)
				require.Equal(t, updateReq.Routes, rsp.Routes)
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
			require.Equal(t, testSuccessCode, resp.Code, "delete should return success")
		})

		t.Run("delete_removes_partial_menu_references", func(t *testing.T) {
			resp, err = cli.Create(&authz.Menu{
				ParentID: "root",
				Label:    "Partial Menu",
				Path:     "/partial-menu",
			})
			require.NoError(t, err)
			var partialMenuID string
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				partialMenuID = rsp.ID
			})

			var cliRole *client.Client
			cliRole, err = client.New(roleAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}))
			require.NoError(t, err)
			resp, err = cliRole.Create(&authz.Role{
				Name:           "Partial Menu Role",
				Code:           "partial_menu_role",
				MenuPartialIDs: []string{partialMenuID},
			})
			require.NoError(t, err)
			var partialRoleID string
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				partialRoleID = rsp.ID
			})

			resp, err = cli.Delete(partialMenuID)
			require.NoError(t, err)
			require.NotNil(t, resp)

			got := new(authz.Role)
			resp, err = cliRole.Get(partialRoleID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotContains(t, []string(rsp.MenuPartialIDs), partialMenuID)
			})

			resp, err = cliRole.Delete(partialRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
		})

		t.Run("invalid_user_role_does_not_fallback_to_default_role", func(t *testing.T) {
			resp, err = cli.Create(&authz.Menu{
				ParentID: "root",
				Label:    "Default Fallback Menu",
				Path:     "/default-fallback-menu",
			})
			require.NoError(t, err)
			var defaultMenuID string
			helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				defaultMenuID = rsp.ID
			})

			var cliRole *client.Client
			cliRole, err = client.New(roleAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}))
			require.NoError(t, err)
			defaultRole := true
			resp, err = cliRole.Create(&authz.Role{
				Name:    "Default Fallback Role",
				Code:    "default_fallback_role",
				Default: &defaultRole,
				MenuIDs: []string{defaultMenuID},
			})
			require.NoError(t, err)
			var defaultRoleID string
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				defaultRoleID = rsp.ID
			})

			missingRoleID := "missing_default_fallback_role"
			invalidUserRole := &authz.UserRole{
				UserID: userID,
				RoleID: missingRoleID,
				Base:   model.Base{ID: "invalid_default_fallback_user_role"},
			}
			require.NoError(t, database.Database[*authz.UserRole](context.Background()).WithoutHook().Create(invalidUserRole))
			_, err = rbac.Enforcer.AddRoleForUser(userID, missingRoleID)
			require.NoError(t, err)
			_, err = rbac.Enforcer.AddPermissionForUser(missingRoleID, "/api/menus", http.MethodGet, "allow")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = database.Database[*authz.UserRole](context.Background()).WithoutHook().WithPurge().Delete(invalidUserRole)
				_, _ = rbac.Enforcer.DeleteRoleForUser(userID, missingRoleID)
				_, _ = rbac.Enforcer.DeletePermissionForUser(missingRoleID, "/api/menus", http.MethodGet, "allow")
				_, _ = cliRole.Delete(defaultRoleID)
				_, _ = cli.Delete(defaultMenuID)
			})

			var userMenuCli *client.Client
			userMenuCli, err = client.New(menuAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: userSessionID,
			}))
			require.NoError(t, err)
			items := make([]*authz.Menu, 0)
			total := new(int64)
			resp, err = userMenuCli.List(&items, total)
			require.NoError(t, err)
			helper.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				requireNoMenu(t, rsp.Items, defaultMenuID)
			})
		})
	})

	t.Run("role", func(t *testing.T) {
		cli, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var roleID string
		var conflictRoleID string
		var resp *client.Resp
		var roleMenuID string

		cliMenu, err := client.New(menuAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		resp, err = cliMenu.Create(&authz.Menu{
			ParentID: "root",
			Label:    "Role Test Menu",
			Path:     "/role-test",
			Routes: []authz.Route{
				{Path: "/api/authz/roles", Methods: []string{http.MethodGet}},
			},
		})
		require.NoError(t, err)
		helper.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			roleMenuID = rsp.ID
		})

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
				Name:    "Test Role",
				Code:    "test_role",
				MenuIDs: []string{roleMenuID},
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
			policies, policyErr := rbac.Enforcer.GetPermissionsForUser(createReq.Code)
			require.NoError(t, policyErr)
			requirePolicy(t, policies, createReq.Code, "/api/authz/roles", http.MethodGet, "allow")
			requireNoPolicy(t, policies, createReq.Code, "/api/authz/roles", http.MethodPost, "allow")
		})

		t.Run("create_conflict_role", func(t *testing.T) {
			resp, err = cli.Create(&authz.Role{Name: "Test Role Conflict", Code: "test_role_conflict"})
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				conflictRoleID = rsp.ID
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
				Name:    "Test Role Updated",
				Code:    "test_role",
				MenuIDs: []string{roleMenuID},
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

		t.Run("update_code_forbidden", func(t *testing.T) {
			_, err = cli.Update(roleID, &authz.Role{
				Name: "Test Role Code Changed",
				Code: "test_role_updated",
			})
			require.Error(t, err)

			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, "test_role", rsp.Code)
			})
		})

		t.Run("failed_update_keeps_existing_policy", func(t *testing.T) {
			_, err = cli.Update(roleID, &authz.Role{
				Name:    "Test Role Conflict",
				Code:    "test_role",
				MenuIDs: nil,
			})
			require.Error(t, err)

			policies, policyErr := rbac.Enforcer.GetPermissionsForUser("test_role")
			require.NoError(t, policyErr)
			requirePolicy(t, policies, "test_role", "/api/authz/roles", http.MethodGet, "allow")
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.Role{Name: "Test Role Patched"}
			resp, err = cli.Patch(roleID, patchReq)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, patchReq.Name, rsp.Name)
				require.Equal(t, "test_role", rsp.Code)
			})
		})

		t.Run("patch_code_forbidden", func(t *testing.T) {
			_, err = cli.Patch(roleID, &authz.Role{Code: "test_role_patched"})
			require.Error(t, err)

			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, "test_role", rsp.Code)
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
			require.Equal(t, testSuccessCode, resp.Code, "delete should return success")
		})

		resp, err = cli.Delete(conflictRoleID)
		require.NoError(t, err)
		require.NotNil(t, resp)

		resp, err = cliMenu.Delete(roleMenuID)
		require.NoError(t, err)
		require.NotNil(t, resp)
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

		t.Run("delete_role_cleans_user_roles", func(t *testing.T) {
			resp, err = cliRole.Create(&authz.Role{Name: "Deleted Role", Code: "deleted_role"})
			require.NoError(t, err)
			var deletedRoleID string
			helper.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				deletedRoleID = rsp.ID
			})

			resp, err = cli.Create(&authz.UserRole{
				UserID: userID,
				RoleID: deletedRoleID,
			})
			require.NoError(t, err)
			helper.TestResp[*authz.UserRole](t, resp, func(t *testing.T, rsp *authz.UserRole) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
			})

			resp, err = cliRole.Delete(deletedRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)

			remaining := make([]*authz.UserRole, 0)
			err = database.Database[*authz.UserRole](context.Background()).
				WithQuery(&authz.UserRole{RoleID: deletedRoleID}).
				List(&remaining)
			require.NoError(t, err)
			require.Empty(t, remaining)
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(userRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, testSuccessCode, resp.Code, "delete should return success")
		})
	})
}

func requireRoute(t *testing.T, routes []authz.Route, path string, methods []string) {
	t.Helper()
	for _, route := range routes {
		if route.Path == path {
			require.Equal(t, methods, route.Methods)
			return
		}
	}
	require.Failf(t, "route not found", "path: %s", path)
}

func requireNoRoute(t *testing.T, routes []authz.Route, path string) {
	t.Helper()
	for _, route := range routes {
		require.NotEqual(t, path, route.Path)
	}
}

func requireNoMenu(t *testing.T, menus []*authz.Menu, menuID string) {
	t.Helper()
	for _, menu := range menus {
		require.NotEqual(t, menuID, menu.ID)
	}
}

func requirePolicy(t *testing.T, policies [][]string, want ...string) {
	t.Helper()
	for _, policy := range policies {
		if equalStrings(policy, want) {
			return
		}
	}
	require.Failf(t, "policy not found", "policy: %v", want)
}

func requireNoPolicy(t *testing.T, policies [][]string, want ...string) {
	t.Helper()
	for _, policy := range policies {
		require.Falsef(t, equalStrings(policy, want), "unexpected policy: %v", want)
	}
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
