package authz_test

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
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

const testSuccessCode = 0

var (
	token        = "-"
	port         = testutil.SetupRandomServerPort()
	rootUsername = "root"
	rootPassword = "12345678"

	signupAPI = testutil.URL(port, "/api/signup")
	loginAPI  = testutil.URL(port, "/api/login")

	routesAPI      = testutil.URL(port, "/api/authz/routes")
	menuAPI        = testutil.URL(port, "/api/authz/menus")
	roleAPI        = testutil.URL(port, "/api/authz/roles")
	roleBindingAPI = testutil.URL(port, "/api/authz/role-bindings")

	userAdminAPI = testutil.URL(port, "/api/iam/admin/users")

	tenantHeader    = "X-Tenant-ID"
	tenantUserAgent = "gst-authz-tenant-test"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_USERNAME, "test_module")
	os.Setenv(config.MYSQL_PASSWORD, "test_module")
	os.Setenv(config.MYSQL_DATABASE, "test_module")
	os.Setenv(config.REDIS_ENABLED, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLED, "true")
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
	authz.Register(authz.Config{
		TenantResolver: authz.HeaderTenantResolver(tenantHeader),
	})
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

func TestAuthzRoutes(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)

	t.Run("list", func(t *testing.T) {
		cli, err := client.New(routesAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp authz.RoutesRsp) {
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
			requireRoute(t, rsp.Items, "/api/authz/routes", []string{http.MethodGet})
			requireRoute(t, rsp.Items, "/api/authz/roles", []string{http.MethodGet, http.MethodPost})
			requireRoute(t, rsp.Items, "/api/authz/roles/{id}", []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete})
			requireRoute(t, rsp.Items, "/api/authz/role-bindings", []string{http.MethodGet, http.MethodPost})
			requireRoute(t, rsp.Items, "/api/authz/role-bindings/{id}", []string{http.MethodGet, http.MethodDelete})
			requireRoute(t, rsp.Items, "/api/authz/menus", []string{http.MethodGet, http.MethodPost})
			requireRoute(t, rsp.Items, "/api/authz/menus/{id}", []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete})
		})
	})

	t.Run("uses_header_tenant", func(t *testing.T) {
		tenantA := authzTestUsername("tenant_routes_a")
		tenantB := authzTestUsername("tenant_routes_b")
		userID, userSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_routes_user"), "12345678", tenantUserAgent)
		roleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_routes_role"))
		authzBindTenantRole(t, tenantA, userID, roleID)
		authzGrantTenantPolicy(t, tenantA, roleID, "/api/authz/routes", http.MethodGet)

		cli, err := authzTenantClient(routesAPI, userSessionID, tenantA)
		require.NoError(t, err)
		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp authz.RoutesRsp) {
			t.Helper()
			requireRoute(t, rsp.Items, "/api/authz/routes", []string{http.MethodGet})
		})

		cli, err = authzTenantClient(routesAPI, userSessionID, tenantB)
		require.NoError(t, err)
		_, err = cli.Request(http.MethodGet, nil)
		require.Error(t, err)
	})
}

func TestAuthzMenu(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)
	userID, userSessionID := authzSignupAndLoginUser(t, authzTestUsername("authz_menu_user"), "12345678")

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
			total := new(int)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
			})
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.Menu{
				ParentID: "root",
				Label:    "Test Menu",
				Path:     "/test",
				Routes: []authz.Route{
					{Path: "/api/authz/routes", Methods: []string{http.MethodGet}},
				},
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
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
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, "Test Menu", rsp.Label)
				require.Equal(t, "/test", rsp.Path)
				require.Equal(t, []authz.Route{{Path: "/api/authz/routes", Methods: []string{http.MethodGet}}}, []authz.Route(rsp.Routes))
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Menu{
				ParentID: "root",
				Label:    "Test Menu Updated",
				Path:     "/test-updated",
				Routes: []authz.Route{
					{Path: "/api/authz/routes", Methods: []string{http.MethodGet}},
					{Path: "/api/authz/roles", Methods: []string{http.MethodGet}},
				},
			}
			resp, err = cli.Update(menuID, updateReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, updateReq.Label, rsp.Label)
				require.Equal(t, updateReq.Path, rsp.Path)
				require.Equal(t, updateReq.Routes, rsp.Routes)
			})
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := map[string]string{"label": "Test Menu Patched"}
			resp, err = cli.Patch(menuID, patchReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.Equal(t, menuID, rsp.ID)
				require.Equal(t, patchReq["label"], rsp.Label)
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
			total := new(int)
			resp, err = cliExpand.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
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
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
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
				Base:           model.Base{ID: "partial_menu_role"},
				Code:           "partial_menu_role",
				MenuPartialIDs: []string{partialMenuID},
			})
			require.NoError(t, err)
			var partialRoleID string
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
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
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotContains(t, []string(rsp.MenuPartialIDs), partialMenuID)
			})

			resp, err = cliRole.Delete(partialRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
		})

		t.Run("invalid_role_binding_does_not_fallback_to_default_role", func(t *testing.T) {
			resp, err = cli.Create(&authz.Menu{
				ParentID: "root",
				Label:    "Default Fallback Menu",
				Path:     "/default-fallback-menu",
			})
			require.NoError(t, err)
			var defaultMenuID string
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
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
				Base:    model.Base{ID: "default_fallback_role"},
				Code:    "default_fallback_role",
				Default: &defaultRole,
				MenuIDs: []string{defaultMenuID},
			})
			require.NoError(t, err)
			var defaultRoleID string
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				defaultRoleID = rsp.ID
			})

			missingRoleID := "missing_default_fallback_role"
			invalidRoleBinding := &authz.RoleBinding{
				TenantID:  rbac.DefaultTenant,
				SubjectID: userID,
				RoleID:    missingRoleID,
				Base:      model.Base{ID: util.HashID(userID, missingRoleID)},
			}
			require.NoError(t, database.Database[*authz.RoleBinding](context.Background()).WithoutHook().Create(invalidRoleBinding))
			rbacPolicy := rbac.RBAC()
			rbacCtx := context.Background()
			require.NoError(t, rbacPolicy.AssignRole(rbacCtx, rbac.DefaultTenant, userID, missingRoleID))
			require.NoError(t, rbacPolicy.GrantPermission(rbacCtx, rbac.DefaultTenant, missingRoleID, "/api/authz/menus", http.MethodGet))
			t.Cleanup(func() {
				_ = database.Database[*authz.RoleBinding](context.Background()).WithoutHook().WithPurge().Delete(invalidRoleBinding)
				_ = rbacPolicy.UnassignRole(context.Background(), rbac.DefaultTenant, userID, missingRoleID)
				_ = rbacPolicy.RevokePermission(context.Background(), rbac.DefaultTenant, missingRoleID, "/api/authz/menus", http.MethodGet)
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
			total := new(int)
			resp, err = userMenuCli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				requireNoMenu(t, rsp.Items, defaultMenuID)
			})
		})

		t.Run("list_uses_current_tenant_roles", func(t *testing.T) {
			tenantA := authzTestUsername("tenant_menu_a")
			tenantB := authzTestUsername("tenant_menu_b")
			tenantUserID, tenantUserSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_menu_user"), "12345678", tenantUserAgent)
			resp, err = cli.Create(&authz.Menu{
				ParentID: "root",
				Label:    "Tenant A Menu",
				Path:     "/tenant-a-menu",
				Routes: []authz.Route{
					{Path: "/api/authz/menus", Methods: []string{http.MethodGet}},
				},
			})
			require.NoError(t, err)
			var tenantAMenuID string
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				tenantAMenuID = rsp.ID
			})
			t.Cleanup(func() {
				_, _ = cli.Delete(tenantAMenuID)
			})

			resp, err = cli.Create(&authz.Menu{
				ParentID: "root",
				Label:    "Tenant B Menu",
				Path:     "/tenant-b-menu",
				Routes: []authz.Route{
					{Path: "/api/authz/menus", Methods: []string{http.MethodGet}},
				},
			})
			require.NoError(t, err)
			var tenantBMenuID string
			testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				tenantBMenuID = rsp.ID
			})
			t.Cleanup(func() {
				_, _ = cli.Delete(tenantBMenuID)
			})

			tenantARoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_menu_a_role"), tenantAMenuID)
			authzBindTenantRole(t, tenantA, tenantUserID, tenantARoleID)
			tenantBRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_menu_b_role"), tenantBMenuID)
			authzBindTenantRole(t, tenantB, tenantUserID, tenantBRoleID)

			userMenuCli, err := authzTenantClient(menuAPI, tenantUserSessionID, tenantA)
			require.NoError(t, err)
			items := make([]*authz.Menu, 0)
			total := new(int)
			resp, err = userMenuCli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				requireMenu(t, rsp.Items, tenantAMenuID)
				requireNoMenu(t, rsp.Items, tenantBMenuID)
			})

			userMenuCli, err = authzTenantClient(menuAPI, tenantUserSessionID, tenantB)
			require.NoError(t, err)
			items = make([]*authz.Menu, 0)
			total = new(int)
			resp, err = userMenuCli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Menu]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Menu]) {
				t.Helper()
				requireMenu(t, rsp.Items, tenantBMenuID)
				requireNoMenu(t, rsp.Items, tenantAMenuID)
			})
		})
	})
}

func TestAuthzRole(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)

	t.Run("role", func(t *testing.T) {
		cli, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var roleID string
		var roleCode string
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
		testutil.TestResp[*authz.Menu](t, resp, func(t *testing.T, rsp *authz.Menu) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			roleMenuID = rsp.ID
		})

		t.Run("list", func(t *testing.T) {
			items := make([]*authz.Role, 0)
			total := new(int)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Role]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Role]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
			})
		})

		t.Run("create_requires_id", func(t *testing.T) {
			_, err = cli.Create(&authz.Role{
				Code: "missing_id_role",
			})
			require.Error(t, err)
		})

		t.Run("create_rejects_system_root_id", func(t *testing.T) {
			_, err = cli.Create(&authz.Role{
				Base: model.Base{ID: consts.AUTHZ_SYSTEM_ROLE_ROOT},
				Code: consts.AUTHZ_SYSTEM_ROLE_ROOT,
			})
			require.Error(t, err)
		})

		t.Run("create", func(t *testing.T) {
			roleID = authzTestUsername("test_role")
			createReq := &authz.Role{
				Base:    model.Base{ID: roleID},
				MenuIDs: []string{roleMenuID},
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, rbac.DefaultTenant, rsp.TenantID)
				require.Equal(t, roleID, rsp.Code)
				roleID = rsp.ID
				roleCode = rsp.Code
			})
			requireCasbinPolicy(t, rbac.DefaultTenant, roleID, "/api/authz/roles", http.MethodGet, "allow")
			requireNoCasbinPolicy(t, rbac.DefaultTenant, roleID, "/api/authz/roles", http.MethodPost, "allow")
		})

		t.Run("get", func(t *testing.T) {
			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, roleCode, rsp.Code)
			})
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Role{
				Code:    authzTestUsername("test_role_updated"),
				MenuIDs: []string{roleMenuID},
			}
			resp, err = cli.Update(roleID, updateReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, updateReq.Code, rsp.Code)
				roleCode = rsp.Code
			})
		})

		t.Run("update_code_preserves_role_id_policies", func(t *testing.T) {
			nextCode := authzTestUsername("test_role_updated_again")
			resp, err = cli.Update(roleID, &authz.Role{
				Code:    nextCode,
				MenuIDs: []string{roleMenuID},
			})
			require.NoError(t, err)

			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, nextCode, rsp.Code)
				roleCode = rsp.Code
			})

			requireCasbinPolicy(t, rbac.DefaultTenant, roleID, "/api/authz/roles", http.MethodGet, "allow")

			requireNoCasbinPolicy(t, rbac.DefaultTenant, nextCode, "/api/authz/roles", http.MethodGet, "allow")
		})

		t.Run("failed_tenant_update_keeps_existing_policy", func(t *testing.T) {
			_, err = cli.Update(roleID, &authz.Role{
				TenantID: "other",
				Code:     roleCode,
				MenuIDs:  nil,
			})
			require.Error(t, err)

			requireCasbinPolicy(t, rbac.DefaultTenant, roleID, "/api/authz/roles", http.MethodGet, "allow")
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.Role{Code: roleCode}
			resp, err = cli.Patch(roleID, patchReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, roleID, rsp.ID)
				require.Equal(t, roleCode, rsp.Code)
			})
		})

		t.Run("patch_code", func(t *testing.T) {
			nextCode := authzTestUsername("test_role_patched")
			resp, err = cli.Patch(roleID, &authz.Role{Code: nextCode})
			require.NoError(t, err)

			got := new(authz.Role)
			resp, err = cli.Get(roleID, got)
			require.NoError(t, err)
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.Equal(t, nextCode, rsp.Code)
				roleCode = rsp.Code
			})
		})

		t.Run("list_expand", func(t *testing.T) {
			items := make([]*authz.Role, 0)
			total := new(int)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.Role]](t, resp, func(t *testing.T, rsp ListResponse[*authz.Role]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
			})
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(roleID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, testSuccessCode, resp.Code, "delete should return success")
		})

		resp, err = cliMenu.Delete(roleMenuID)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
}

func TestAuthzRoleBinding(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)
	userID, _ := authzSignupAndLoginUser(t, authzTestUsername("authz_role_binding_user"), "12345678")

	t.Run("role_binding", func(t *testing.T) {
		cli, err := client.New(roleBindingAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		var roleBindingID string
		var roleID string
		var resp *client.Resp

		// Create a role for assigning to user (role from previous test was deleted).
		cliRole, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)
		resp, err = cliRole.Create(&authz.Role{
			Base: model.Base{ID: "role_binding_test_role"},
			Code: "role_binding_test_role",
		})
		require.NoError(t, err)
		testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			roleID = rsp.ID
		})

		t.Run("list", func(t *testing.T) {
			items := make([]*authz.RoleBinding, 0)
			total := new(int)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.RoleBinding]](t, resp, func(t *testing.T, rsp ListResponse[*authz.RoleBinding]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
			})
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.RoleBinding{
				SubjectID: userID,
				RoleID:    roleID,
			}
			resp, err = cli.Create(createReq)
			require.NoError(t, err)
			testutil.TestResp[*authz.RoleBinding](t, resp, func(t *testing.T, rsp *authz.RoleBinding) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				require.Equal(t, rbac.DefaultTenant, rsp.TenantID)
				require.Equal(t, userID, rsp.SubjectID)
				require.Equal(t, roleID, rsp.RoleID)
				roleBindingID = rsp.ID
			})
			requireCasbinGroupingPolicy(t, userID, roleID, rbac.DefaultTenant)
		})

		t.Run("get", func(t *testing.T) {
			got := new(authz.RoleBinding)
			resp, err = cli.Get(roleBindingID, got)
			require.NoError(t, err)
			testutil.TestResp[*authz.RoleBinding](t, resp, func(t *testing.T, rsp *authz.RoleBinding) {
				t.Helper()
				require.Equal(t, roleBindingID, rsp.ID)
				require.Equal(t, rbac.DefaultTenant, rsp.TenantID)
				require.Equal(t, userID, rsp.SubjectID)
				require.Equal(t, roleID, rsp.RoleID)
			})
		})

		t.Run("list_expand", func(t *testing.T) {
			items := make([]*authz.RoleBinding, 0)
			total := new(int)
			resp, err = cli.List(&items, total)
			require.NoError(t, err)
			testutil.TestResp[ListResponse[*authz.RoleBinding]](t, resp, func(t *testing.T, rsp ListResponse[*authz.RoleBinding]) {
				t.Helper()
				require.NotNil(t, rsp.Items)
				require.GreaterOrEqual(t, rsp.Total, 0)
			})
		})

		t.Run("delete_role_cleans_role_bindings", func(t *testing.T) {
			resp, err = cliRole.Create(&authz.Role{
				Base: model.Base{ID: "deleted_role"},
				Code: "deleted_role",
			})
			require.NoError(t, err)
			var deletedRoleID string
			testutil.TestResp[*authz.Role](t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
				deletedRoleID = rsp.ID
			})

			resp, err = cli.Create(&authz.RoleBinding{
				SubjectID: userID,
				RoleID:    deletedRoleID,
			})
			require.NoError(t, err)
			testutil.TestResp[*authz.RoleBinding](t, resp, func(t *testing.T, rsp *authz.RoleBinding) {
				t.Helper()
				require.NotEmpty(t, rsp.ID)
			})

			resp, err = cliRole.Delete(deletedRoleID)
			require.NoError(t, err)
			require.NotNil(t, resp)

			remaining := make([]*authz.RoleBinding, 0)
			err = database.Database[*authz.RoleBinding](context.Background()).
				WithQuery(&authz.RoleBinding{TenantID: rbac.DefaultTenant, RoleID: deletedRoleID}).
				List(&remaining)
			require.NoError(t, err)
			require.Empty(t, remaining)
		})

		t.Run("delete", func(t *testing.T) {
			resp, err = cli.Delete(roleBindingID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, testSuccessCode, resp.Code, "delete should return success")
		})
	})

	t.Run("subjects_in_tenant", func(t *testing.T) {
		requireRBACSubjectsInTenant(t)
	})
}

func TestIAMUserStatusTenantAuthorization(t *testing.T) {
	tenantA := authzTestUsername("tenant_iam_a")
	tenantB := authzTestUsername("tenant_iam_b")
	adminUserID, adminSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_iam_admin"), "12345678", tenantUserAgent)
	targetTenantAUserID := authzSignupUser(t, authzTestUsername("tenant_iam_target_a"), "12345678")
	targetTenantBUserID := authzSignupUser(t, authzTestUsername("tenant_iam_target_b"), "12345678")

	adminRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_iam_admin_role"))
	authzBindTenantRole(t, tenantA, adminUserID, adminRoleID)
	authzGrantTenantPolicy(t, tenantA, adminRoleID, "/api/iam/admin/users/{id}/status", http.MethodPatch)
	tenantAMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_iam_member_a_role"))
	authzBindTenantRole(t, tenantA, targetTenantAUserID, tenantAMemberRoleID)
	tenantBMemberRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_iam_member_b_role"))
	authzBindTenantRole(t, tenantB, targetTenantBUserID, tenantBMemberRoleID)
	rootMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_iam_root_member_role"))
	authzBindTenantRole(t, tenantA, rootUsername, rootMemberRoleID)

	cli, err := authzTenantClient(userAdminAPI, adminSessionID, tenantA)
	require.NoError(t, err)
	resp, err := cli.Patch(targetTenantAUserID+"/status", iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.NoError(t, err)
	testutil.TestResp[iam.UserStatusPatchRsp](t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
		t.Helper()
		require.NotEmpty(t, rsp.Msg)
	})

	_, err = cli.Patch(rootUsername+"/status", iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")

	_, err = cli.Patch(targetTenantBUserID+"/status", iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.Error(t, err)

	cli, err = authzTenantClient(userAdminAPI, adminSessionID, tenantB)
	require.NoError(t, err)
	_, err = cli.Patch(targetTenantAUserID+"/status", iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.Error(t, err)
}

func TestIAMAdminUserTenantListGet(t *testing.T) {
	tenantA := authzTestUsername("tenant_admin_users_a")
	tenantB := authzTestUsername("tenant_admin_users_b")
	adminUserID, adminSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_admin_users_admin"), "12345678", tenantUserAgent)
	targetTenantAUserID := authzSignupUser(t, authzTestUsername("tenant_admin_users_target_a"), "12345678")
	targetTenantBUserID := authzSignupUser(t, authzTestUsername("tenant_admin_users_target_b"), "12345678")

	adminRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_admin_users_admin_role"))
	authzBindTenantRole(t, tenantA, adminUserID, adminRoleID)
	authzGrantTenantPolicy(t, tenantA, adminRoleID, "/api/iam/admin/users", http.MethodGet)
	authzGrantTenantPolicy(t, tenantA, adminRoleID, "/api/iam/admin/users/{id}", http.MethodGet)
	tenantAMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_admin_users_member_a_role"))
	authzBindTenantRole(t, tenantA, targetTenantAUserID, tenantAMemberRoleID)
	tenantBMemberRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_admin_users_member_b_role"))
	authzBindTenantRole(t, tenantB, targetTenantBUserID, tenantBMemberRoleID)
	rootMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_admin_users_root_member_role"))
	authzBindTenantRole(t, tenantA, rootUsername, rootMemberRoleID)

	cli, err := authzTenantClient(userAdminAPI, adminSessionID, tenantA)
	require.NoError(t, err)

	t.Run("list_tenant_users", func(t *testing.T) {
		items := make([]iam.AdminUserView, 0)
		total := new(int)
		_, listErr := cli.List(&items, total)
		require.NoError(t, listErr)
		require.Positive(t, *total)
		requireAdminUserView(t, items, adminUserID)
		requireAdminUserView(t, items, targetTenantAUserID)
		requireNoAdminUserView(t, items, targetTenantBUserID)
		requireNoAdminUserView(t, items, rootUsername)
	})

	t.Run("get_tenant_user", func(t *testing.T) {
		got := new(iam.AdminUserGetRsp)
		_, getErr := cli.Get(targetTenantAUserID, got)
		require.NoError(t, getErr)
		require.Equal(t, targetTenantAUserID, got.User.ID)
	})

	t.Run("get_other_tenant_user_forbidden", func(t *testing.T) {
		got := new(iam.AdminUserGetRsp)
		_, getErr := cli.Get(targetTenantBUserID, got)
		require.Error(t, getErr)
	})
}

func requireRBACSubjectsInTenant(t *testing.T) {
	t.Helper()

	tenantA := authzTestUsername("tenant_subjects_a")
	tenantB := authzTestUsername("tenant_subjects_b")
	userAID := authzSignupUser(t, authzTestUsername("tenant_subjects_user_a"), "12345678")
	userBID := authzSignupUser(t, authzTestUsername("tenant_subjects_user_b"), "12345678")

	tenantARoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_subjects_a_role"))
	tenantASecondRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_subjects_a_second_role"))
	tenantBRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_subjects_b_role"))
	authzBindTenantRole(t, tenantA, userAID, tenantARoleID)
	authzBindTenantRole(t, tenantA, userAID, tenantASecondRoleID)
	authzBindTenantRole(t, tenantB, userBID, tenantBRoleID)

	subjects, err := rbac.RBAC().SubjectsInTenant(context.Background(), tenantA)
	require.NoError(t, err)
	require.Contains(t, subjects, userAID)
	require.NotContains(t, subjects, userBID)
	require.Len(t, filterSubjects(subjects, userAID), 1)
}

func TestIAMLoginStoresSessionTenant(t *testing.T) {
	tenantID := authzTestUsername("tenant_login")
	username := authzTestUsername("tenant_login_user")
	password := "12345678"
	userID := authzSignupUser(t, username, password)
	roleID := authzCreateTenantRole(t, tenantID, authzTestUsername("tenant_login_role"))
	authzBindTenantRole(t, tenantID, userID, roleID)

	sessionID := loginSessionIDFromCookieWithUserAgent(t, iam.LoginReq{
		Username: username,
		Password: password,
		TenantID: tenantID,
	}, tenantUserAgent)

	session, err := redis.Cache[modeliamsession.Session]().
		WithContext(t.Context()).
		Get(modeliamsession.SessionIDKey(sessionID))
	require.NoError(t, err)
	require.Equal(t, tenantID, session.TenantID)
}

func TestIAMLoginRejectsTenantOutsideMembership(t *testing.T) {
	username := authzTestUsername("tenant_login_forbidden_user")
	password := "12345678"
	authzSignupUser(t, username, password)

	cli, err := client.New(loginAPI, client.WithUserAgent(tenantUserAgent))
	require.NoError(t, err)

	_, err = cli.Create(iam.LoginReq{
		Username: username,
		Password: password,
		TenantID: authzTestUsername("tenant_login_forbidden"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}

func authzAdminSessionID(t *testing.T) string {
	t.Helper()

	return loginSessionIDFromCookie(t, iam.LoginReq{
		Username: rootUsername,
		Password: rootPassword,
	})
}

func authzSignupAndLoginUser(t *testing.T, username, password string) (string, string) {
	t.Helper()

	userID := authzSignupUser(t, username, password)
	sessionID := loginSessionIDFromCookie(t, iam.LoginReq{
		Username: username,
		Password: password,
	})
	return userID, sessionID
}

func authzSignupAndLoginUserWithUserAgent(t *testing.T, username, password, userAgent string) (string, string) {
	t.Helper()

	userID := authzSignupUser(t, username, password)
	sessionID := loginSessionIDFromCookieWithUserAgent(t, iam.LoginReq{
		Username: username,
		Password: password,
	}, userAgent)
	return userID, sessionID
}

func authzSignupUser(t *testing.T, username, password string) string {
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

func authzTestUsername(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	return loginSessionIDFromCookieWithUserAgent(t, reqPayload, "")
}

func loginSessionIDFromCookieWithUserAgent(t *testing.T, reqPayload iam.LoginReq, userAgent string) string {
	t.Helper()

	options := make([]client.Option, 0, 1)
	if userAgent != "" {
		options = append(options, client.WithUserAgent(userAgent))
	}
	cli, err := client.New(loginAPI, options...)
	require.NoError(t, err)

	apiResp, err := cli.Create(reqPayload)
	require.NoError(t, err)

	testutil.TestResp(t, apiResp, func(t *testing.T, rsp iam.LoginRsp) {
		t.Helper()
		require.False(t, rsp.ServerTime.IsZero())
		require.False(t, rsp.Session.ExpiresAt.IsZero())
		if reqPayload.TenantID != "" {
			require.Equal(t, reqPayload.TenantID, rsp.Session.TenantID)
		}
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

func authzTenantClient(api, sessionID, tenantID string) (*client.Client, error) {
	return client.New(
		api,
		client.WithHeader(http.Header{
			tenantHeader: []string{tenantID},
		}),
		client.WithUserAgent(tenantUserAgent),
		client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}),
	)
}

func authzCreateTenantRole(t *testing.T, tenantID, code string, menuIDs ...string) string {
	t.Helper()

	role := &authz.Role{
		Base:     model.Base{ID: util.HashID(tenantID, code)},
		TenantID: tenantID,
		Code:     code,
		MenuIDs:  menuIDs,
	}
	require.NoError(t, database.Database[*authz.Role](context.Background()).Create(role))
	t.Cleanup(func() {
		_ = database.Database[*authz.Role](context.Background()).WithPurge().Delete(role)
	})
	return role.ID
}

func authzBindTenantRole(t *testing.T, tenantID, subjectID, roleID string) {
	t.Helper()

	roleBinding := &authz.RoleBinding{
		TenantID:  tenantID,
		SubjectID: subjectID,
		RoleID:    roleID,
	}
	require.NoError(t, database.Database[*authz.RoleBinding](context.Background()).Create(roleBinding))
	t.Cleanup(func() {
		_ = database.Database[*authz.RoleBinding](context.Background()).WithPurge().Delete(roleBinding)
	})
}

func authzGrantTenantPolicy(t *testing.T, tenantID, roleID, object, action string) {
	t.Helper()

	require.NoError(t, rbac.RBAC().GrantPermission(context.Background(), tenantID, roleID, object, action))
}

func filterSubjects(subjects []string, target string) []string {
	matched := make([]string, 0, 1)
	for _, subject := range subjects {
		if subject == target {
			matched = append(matched, subject)
		}
	}
	return matched
}

func requireAdminUserView(t *testing.T, users []iam.AdminUserView, userID string) iam.AdminUserView {
	t.Helper()

	for _, user := range users {
		if user.ID == userID {
			return user
		}
	}
	require.Failf(t, "admin user view not found", "user_id=%s", userID)
	return iam.AdminUserView{}
}

func requireNoAdminUserView(t *testing.T, users []iam.AdminUserView, userID string) {
	t.Helper()

	for _, user := range users {
		require.NotEqual(t, userID, user.ID)
	}
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

func requireMenu(t *testing.T, menus []*authz.Menu, menuID string) {
	t.Helper()
	for _, menu := range menus {
		if menu.ID == menuID {
			return
		}
	}
	require.Failf(t, "menu not found", "menu_id: %s", menuID)
}

func requireNoMenu(t *testing.T, menus []*authz.Menu, menuID string) {
	t.Helper()
	for _, menu := range menus {
		require.NotEqual(t, menuID, menu.ID)
	}
}

func requireCasbinPolicy(t *testing.T, tenant, role, object, action, effect string) {
	t.Helper()
	requireCasbinRule(t, "p", tenant, role, object, action, effect)
}

func requireNoCasbinPolicy(t *testing.T, tenant, role, object, action, effect string) {
	t.Helper()
	requireNoCasbinRule(t, "p", tenant, role, object, action, effect)
}

func requireCasbinGroupingPolicy(t *testing.T, subject, role, tenant string) {
	t.Helper()
	requireCasbinRule(t, "g", subject, role, tenant, "", "")
}

func requireCasbinRule(t *testing.T, ptype, v0, v1, v2, v3, v4 string) {
	t.Helper()
	rules := listCasbinRules(t, ptype, v0, v1, v2, v3, v4)
	require.NotEmpty(t, rules)
}

func requireNoCasbinRule(t *testing.T, ptype, v0, v1, v2, v3, v4 string) {
	t.Helper()
	rules := listCasbinRules(t, ptype, v0, v1, v2, v3, v4)
	require.Empty(t, rules)
}

func listCasbinRules(t *testing.T, ptype, v0, v1, v2, v3, v4 string) []*authz.CasbinRule {
	t.Helper()
	rules := make([]*authz.CasbinRule, 0)
	require.NoError(t, database.Database[*authz.CasbinRule](context.Background()).WithQuery(&authz.CasbinRule{
		Ptype: ptype,
		V0:    v0,
		V1:    v1,
		V2:    v2,
		V3:    v3,
		V4:    v4,
	}).List(&rules))
	return rules
}
