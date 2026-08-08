package authz_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/redis"
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

	tenantHeader    = "X-Tenant-ID"
	tenantUserAgent = "gst-authz-tenant-test"
)

const (
	signupPath      = "/api/signup"
	loginPath       = "/api/login"
	routesPath      = "/api/authz/routes"
	menuPath        = "/api/authz/menus"
	rolePath        = "/api/authz/roles"
	roleBindingPath = "/api/authz/role-bindings"
	userAdminPath   = "/api/iam/admin/users"
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
			// The request tenant's one source is CTX_TENANT_ID, written by
			// trusted middleware ahead of Authz. The tests stand in for a
			// trusted gateway: registered after IAMSession, this overwrites the
			// session's tenant with the header's whenever one is sent.
			middleware.RegisterAuth(func(c *gin.Context) {
				if tenantID := strings.TrimSpace(c.GetHeader(tenantHeader)); tenantID != "" {
					c.Set(consts.CTX_TENANT_ID, tenantID)
				}
				c.Next()
			})
			authz.Register()
		},
		Seed: seedBaseline,
	})
}

// seedBaseline creates the baseline rows the tests depend on: the root user
// with its password credential, and the root menu row anchoring the menu
// tree. Baseline data is application data, so the test creates it explicitly
// through the standard database chain.
func seedBaseline() {
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

	rootMenu := &modelauthz.Menu{Base: model.Base{ID: model.RootID}, ParentID: model.RootID}
	if err := database.Database[*modelauthz.Menu](ctx).Create(rootMenu); err != nil {
		panic(err)
	}
}

func TestAuthzRoutes(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)

	t.Run("list", func(t *testing.T) {
		cli := authzSessionClient(t, adminSessionID)

		// #modelauthz.RoutesRsp {
		//   +Items => []modelauthz.Route [
		//     0 => {
		//       +Path    => "/api/authz/roles" #string
		//       +Methods => ["GET", "POST"] #[]string
		//     }
		//   ]
		// }
		rsp, err := client.Get[authz.RoutesRsp](cli, routesPath)
		require.NoError(t, err)

		require.NotEmpty(t, rsp.Items, "routes list should not be empty")
		requireRoute(t, rsp.Items, "/api/authz/routes", []string{http.MethodGet})
		requireRoute(t, rsp.Items, "/api/authz/roles", []string{http.MethodGet, http.MethodPost})
		requireRoute(t, rsp.Items, "/api/authz/roles/{id}", []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete})
		requireRoute(t, rsp.Items, "/api/authz/role-bindings", []string{http.MethodGet, http.MethodPost})
		requireRoute(t, rsp.Items, "/api/authz/role-bindings/{id}", []string{http.MethodGet, http.MethodDelete})
		requireRoute(t, rsp.Items, "/api/authz/menus", []string{http.MethodGet, http.MethodPost})
		requireRoute(t, rsp.Items, "/api/authz/menus/{id}", []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete})
	})

	t.Run("uses_header_tenant", func(t *testing.T) {
		tenantA := authzTestUsername("tenant_routes_a")
		tenantB := authzTestUsername("tenant_routes_b")
		userID, userSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_routes_user"), "12345678", tenantUserAgent)
		roleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_routes_role"))
		authzBindTenantRole(t, tenantA, userID, roleID)
		authzGrantTenantPolicy(t, tenantA, roleID, types.Permission{Object: "/api/authz/routes", Action: http.MethodGet})

		cli := authzTenantClient(t, userSessionID, tenantA)
		rsp, err := client.Get[authz.RoutesRsp](cli, routesPath)
		require.NoError(t, err)
		requireRoute(t, rsp.Items, "/api/authz/routes", []string{http.MethodGet})

		cli = authzTenantClient(t, userSessionID, tenantB)
		_, err = client.Get[authz.RoutesRsp](cli, routesPath)
		require.Error(t, err)
	})
}

func TestAuthzMenu(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)
	userID, userSessionID := authzSignupAndLoginUser(t, authzTestUsername("authz_menu_user"), "12345678")

	t.Run("menu", func(t *testing.T) {
		cli := authzSessionClient(t, adminSessionID)
		var menuID string

		t.Run("list", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.Menu]](cli, menuPath)
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
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
			rsp, err := client.Post[authz.Menu](cli, menuPath, createReq)
			require.NoError(t, err)
			require.NotEmpty(t, rsp.ID)
			require.Equal(t, createReq.Label, rsp.Label)
			require.Equal(t, createReq.Path, rsp.Path)
			require.Equal(t, createReq.ParentID, rsp.ParentID)
			require.Equal(t, createReq.Routes, rsp.Routes)
			menuID = rsp.ID
		})

		t.Run("get", func(t *testing.T) {
			rsp, err := client.Get[authz.Menu](cli, menuPath+"/"+menuID)
			require.NoError(t, err)
			require.Equal(t, menuID, rsp.ID)
			require.Equal(t, "Test Menu", rsp.Label)
			require.Equal(t, "/test", rsp.Path)
			require.Equal(t, []authz.Route{{Path: "/api/authz/routes", Methods: []string{http.MethodGet}}}, []authz.Route(rsp.Routes))
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
			rsp, err := client.Put[authz.Menu](cli, menuPath+"/"+menuID, updateReq)
			require.NoError(t, err)
			require.Equal(t, menuID, rsp.ID)
			require.Equal(t, updateReq.Label, rsp.Label)
			require.Equal(t, updateReq.Path, rsp.Path)
			require.Equal(t, updateReq.Routes, rsp.Routes)
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := map[string]string{"label": "Test Menu Patched"}
			rsp, err := client.Patch[authz.Menu](cli, menuPath+"/"+menuID, patchReq)
			require.NoError(t, err)
			require.Equal(t, menuID, rsp.ID)
			require.Equal(t, patchReq["label"], rsp.Label)
			require.Equal(t, "/test-updated", rsp.Path)
		})

		t.Run("list_expand", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.Menu]](cli, menuPath,
				client.WithExpand("Children,Parent", 1))
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
		})

		t.Run("delete", func(t *testing.T) {
			_, err := client.Delete[struct{}](cli, menuPath+"/"+menuID, nil)
			require.NoError(t, err, "delete should return success")
		})

		// Menus are global and their routes are what every tenant's roles
		// derive permissions from, so writing one is reserved for system-level
		// subjects. An ordinary subject is refused even when a policy grants
		// it the route: the authorization middleware cannot draw this line,
		// because inside its own tenant such a subject passes it.
		t.Run("write_requires_system_subject", func(t *testing.T) {
			guardTenant := authzTestUsername("menu_guard_tenant")
			memberID, memberSessionID := authzSignupAndLoginUserWithUserAgent(
				t, authzTestUsername("menu_guard_user"), "12345678", tenantUserAgent,
			)
			guardRoleID := authzCreateTenantRole(t, guardTenant, authzTestUsername("menu_guard_role"))
			authzBindTenantRole(t, guardTenant, memberID, guardRoleID)
			authzGrantTenantPolicy(t, guardTenant, guardRoleID,
				types.Permission{Object: menuPath, Action: http.MethodPost})

			memberCli := authzTenantClient(t, memberSessionID, guardTenant)
			_, err := client.Post[authz.Menu](memberCli, menuPath, &authz.Menu{
				ParentID: "root",
				Label:    "Guarded Menu",
				Path:     "/guarded-menu",
			})
			testutil.RequireError(t, err, http.StatusForbidden)
		})

		t.Run("delete_removes_menu_references", func(t *testing.T) {
			created, err := client.Post[authz.Menu](cli, menuPath, &authz.Menu{
				ParentID: "root",
				Label:    "Referenced Menu",
				Path:     "/referenced-menu",
			})
			require.NoError(t, err)
			require.NotEmpty(t, created.ID)
			referencedMenuID := created.ID

			role, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name:    "menu_reference_role",
				MenuIDs: []string{referencedMenuID},
			})
			require.NoError(t, err)
			require.NotEmpty(t, role.ID)
			referencedRoleID := role.ID

			_, err = client.Delete[struct{}](cli, menuPath+"/"+referencedMenuID, nil)
			require.NoError(t, err)

			got, err := client.Get[authz.Role](cli, rolePath+"/"+referencedRoleID)
			require.NoError(t, err)
			require.NotContains(t, []string(got.MenuIDs), referencedMenuID)

			_, err = client.Delete[struct{}](cli, rolePath+"/"+referencedRoleID, nil)
			require.NoError(t, err)
		})

		t.Run("invalid_role_binding_does_not_fallback_to_default_role", func(t *testing.T) {
			created, err := client.Post[authz.Menu](cli, menuPath, &authz.Menu{
				ParentID: "root",
				Label:    "Default Fallback Menu",
				Path:     "/default-fallback-menu",
			})
			require.NoError(t, err)
			require.NotEmpty(t, created.ID)
			defaultMenuID := created.ID

			defaultRole := true
			role, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name:    "default_fallback_role",
				Default: &defaultRole,
				MenuIDs: []string{defaultMenuID},
			})
			require.NoError(t, err)
			require.NotEmpty(t, role.ID)

			missingRoleID := "missing_default_fallback_role"
			invalidRoleBinding := &authz.RoleBinding{
				Scope:     tenant.Scope{TenantID: tenant.Default},
				SubjectID: userID,
				RoleID:    missingRoleID,
				Base:      model.Base{ID: util.HashID(userID, missingRoleID)},
			}
			require.NoError(t, database.Database[*authz.RoleBinding](context.Background()).WithoutHook().Create(invalidRoleBinding))
			rbacPolicy := rbac.RBAC()
			rbacCtx := context.Background()
			require.NoError(t, rbacPolicy.AssignRole(rbacCtx, tenant.Default, userID, missingRoleID))
			require.NoError(t, rbacPolicy.SetRolePermissions(rbacCtx, tenant.Default, missingRoleID, []types.Permission{
				{Object: "/api/authz/menus", Action: http.MethodGet},
			}))

			userMenuCli := authzSessionClient(t, userSessionID)
			list, err := client.Get[client.ListResult[*authz.Menu]](userMenuCli, menuPath)
			require.NoError(t, err)
			requireNoMenu(t, list.Items, defaultMenuID)
		})

		t.Run("list_uses_current_tenant_roles", func(t *testing.T) {
			tenantA := authzTestUsername("tenant_menu_a")
			tenantB := authzTestUsername("tenant_menu_b")
			tenantUserID, tenantUserSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("tenant_menu_user"), "12345678", tenantUserAgent)

			menuA, err := client.Post[authz.Menu](cli, menuPath, &authz.Menu{
				ParentID: "root",
				Label:    "Tenant A Menu",
				Path:     "/tenant-a-menu",
				Routes: []authz.Route{
					{Path: "/api/authz/menus", Methods: []string{http.MethodGet}},
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, menuA.ID)
			tenantAMenuID := menuA.ID

			menuB, err := client.Post[authz.Menu](cli, menuPath, &authz.Menu{
				ParentID: "root",
				Label:    "Tenant B Menu",
				Path:     "/tenant-b-menu",
				Routes: []authz.Route{
					{Path: "/api/authz/menus", Methods: []string{http.MethodGet}},
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, menuB.ID)
			tenantBMenuID := menuB.ID

			tenantARoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_menu_a_role"), tenantAMenuID)
			authzBindTenantRole(t, tenantA, tenantUserID, tenantARoleID)
			tenantBRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_menu_b_role"), tenantBMenuID)
			authzBindTenantRole(t, tenantB, tenantUserID, tenantBRoleID)

			userMenuCli := authzTenantClient(t, tenantUserSessionID, tenantA)
			list, err := client.Get[client.ListResult[*authz.Menu]](userMenuCli, menuPath)
			require.NoError(t, err)
			requireMenu(t, list.Items, tenantAMenuID)
			requireNoMenu(t, list.Items, tenantBMenuID)

			userMenuCli = authzTenantClient(t, tenantUserSessionID, tenantB)
			list, err = client.Get[client.ListResult[*authz.Menu]](userMenuCli, menuPath)
			require.NoError(t, err)
			requireMenu(t, list.Items, tenantBMenuID)
			requireNoMenu(t, list.Items, tenantAMenuID)
		})
	})
}

func TestAuthzRole(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)

	t.Run("role", func(t *testing.T) {
		cli := authzSessionClient(t, adminSessionID)
		var roleID string
		var roleName string

		roleMenu, menuErr := client.Post[authz.Menu](cli, menuPath, &authz.Menu{
			ParentID: "root",
			Label:    "Role Test Menu",
			Path:     "/role-test",
			Routes: []authz.Route{
				{Path: "/api/authz/roles", Methods: []string{http.MethodGet}},
			},
		})
		require.NoError(t, menuErr)
		require.NotEmpty(t, roleMenu.ID)
		roleMenuID := roleMenu.ID

		t.Run("list", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.Role]](cli, rolePath)
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
		})

		t.Run("create_requires_name", func(t *testing.T) {
			_, err := client.Post[authz.Role](cli, rolePath, &authz.Role{})
			require.Error(t, err)
		})

		t.Run("create_rejects_system_root", func(t *testing.T) {
			// Both the reserved ID and the reserved name are rejected, so a
			// user-created role can never masquerade as the system role.
			_, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Base: model.Base{ID: consts.AUTHZ_SYSTEM_ROLE_ROOT},
				Name: "some_role_name",
			})
			require.Error(t, err)
			_, err = client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name: consts.AUTHZ_SYSTEM_ROLE_ROOT,
			})
			require.Error(t, err)
		})

		t.Run("rejects_menus_that_do_not_exist", func(t *testing.T) {
			// A dangling menu ID used to be stored as it stood while the
			// permission sync silently expanded only the menus it could find —
			// a role that looked configured with a slice of it missing, and
			// nothing downstream able to see the gap.
			_, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name:    authzTestUsername("dangling_menu_role"),
				MenuIDs: []string{roleMenuID, "no/such/menu"},
			})
			testutil.RequireError(t, err, http.StatusBadRequest)
			require.ErrorContains(t, err, "menus do not exist: no/such/menu")

			created, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name:    authzTestUsername("dangling_menu_role"),
				MenuIDs: []string{roleMenuID},
			})
			require.NoError(t, err)
			_, err = client.Put[authz.Role](cli, rolePath+"/"+created.ID, &authz.Role{
				Name:    created.Name,
				MenuIDs: []string{"no/such/menu"},
			})
			testutil.RequireError(t, err, http.StatusBadRequest)

			_, err = client.Delete[struct{}](cli, rolePath+"/"+created.ID, nil)
			require.NoError(t, err)
		})

		t.Run("create", func(t *testing.T) {
			// The ID is framework-assigned; clients only provide the name.
			roleName = authzTestUsername("test_role")
			createReq := &authz.Role{
				Name:    roleName,
				MenuIDs: []string{roleMenuID},
			}
			rsp, err := client.Post[authz.Role](cli, rolePath, createReq)
			require.NoError(t, err)
			require.NotEmpty(t, rsp.ID)
			require.EqualValues(t, tenant.Default, rsp.TenantID)
			require.Equal(t, roleName, rsp.Name)
			roleID = rsp.ID
			requireCasbinPolicy(t, tenant.Default, roleID, "/api/authz/roles", http.MethodGet, "allow")
			requireNoCasbinPolicy(t, tenant.Default, roleID, "/api/authz/roles", http.MethodPost, "allow")
		})

		t.Run("get", func(t *testing.T) {
			rsp, err := client.Get[authz.Role](cli, rolePath+"/"+roleID)
			require.NoError(t, err)
			require.Equal(t, roleID, rsp.ID)
			require.Equal(t, roleName, rsp.Name)
		})

		t.Run("update", func(t *testing.T) {
			updateReq := &authz.Role{
				Name:    authzTestUsername("test_role_updated"),
				MenuIDs: []string{roleMenuID},
			}
			rsp, err := client.Put[authz.Role](cli, rolePath+"/"+roleID, updateReq)
			require.NoError(t, err)
			require.Equal(t, roleID, rsp.ID)
			require.Equal(t, updateReq.Name, rsp.Name)
			roleName = rsp.Name
		})

		t.Run("update_name_preserves_role_id_policies", func(t *testing.T) {
			nextName := authzTestUsername("test_role_updated_again")
			_, err := client.Put[authz.Role](cli, rolePath+"/"+roleID, &authz.Role{
				Name:    nextName,
				MenuIDs: []string{roleMenuID},
			})
			require.NoError(t, err)

			got, err := client.Get[authz.Role](cli, rolePath+"/"+roleID)
			require.NoError(t, err)
			require.Equal(t, nextName, got.Name)
			roleName = got.Name

			requireCasbinPolicy(t, tenant.Default, roleID, "/api/authz/roles", http.MethodGet, "allow")

			requireNoCasbinPolicy(t, tenant.Default, nextName, "/api/authz/roles", http.MethodGet, "allow")
		})

		// An update naming another tenant cannot move the role: the tenant
		// column is written on insert and never again. This client is
		// system_root, which acts in no single tenant, so there is no scope to
		// compare the named value against and the update is answered rather
		// than refused — a tenant-scoped caller gets tenant.ErrTenantImmutable
		// instead. Either way the role stays where it is, which is what the
		// policies below prove.
		t.Run("tenant_update_cannot_move_the_role", func(t *testing.T) {
			// The menus are carried over so this covers the tenant alone: an
			// update dropping them would revoke the role's permissions on its
			// own, which is the documented replace semantics and not what is
			// under test here.
			current, err := client.Get[authz.Role](cli, rolePath+"/"+roleID)
			require.NoError(t, err)

			_, err = client.Put[authz.Role](cli, rolePath+"/"+roleID, &authz.Role{
				Scope:   tenant.Scope{TenantID: "other"},
				Name:    current.Name,
				MenuIDs: current.MenuIDs,
			})
			require.NoError(t, err)

			moved, err := client.Get[authz.Role](cli, rolePath+"/"+roleID)
			require.NoError(t, err)
			require.EqualValues(t, tenant.Default, moved.TenantID)

			requireCasbinPolicy(t, tenant.Default, roleID, "/api/authz/roles", http.MethodGet, "allow")
			requireNoCasbinPolicy(t, "other", roleID, "/api/authz/roles", http.MethodGet, "allow")
		})

		t.Run("patch", func(t *testing.T) {
			patchReq := &authz.Role{Name: roleName}
			rsp, err := client.Patch[authz.Role](cli, rolePath+"/"+roleID, patchReq)
			require.NoError(t, err)
			require.Equal(t, roleID, rsp.ID)
			require.Equal(t, roleName, rsp.Name)
		})

		t.Run("patch_name", func(t *testing.T) {
			nextName := authzTestUsername("test_role_patched")
			_, err := client.Patch[authz.Role](cli, rolePath+"/"+roleID, &authz.Role{Name: nextName})
			require.NoError(t, err)

			got, err := client.Get[authz.Role](cli, rolePath+"/"+roleID)
			require.NoError(t, err)
			require.Equal(t, nextName, got.Name)
			roleName = got.Name
		})

		t.Run("list_expand", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.Role]](cli, rolePath)
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
		})

		t.Run("delete", func(t *testing.T) {
			_, err := client.Delete[struct{}](cli, rolePath+"/"+roleID, nil)
			require.NoError(t, err, "delete should return success")
		})

		_, menuDelErr := client.Delete[struct{}](cli, menuPath+"/"+roleMenuID, nil)
		require.NoError(t, menuDelErr)
	})
}

func TestAuthzRoleBinding(t *testing.T) {
	adminSessionID := authzAdminSessionID(t)
	userID, _ := authzSignupAndLoginUser(t, authzTestUsername("authz_role_binding_user"), "12345678")

	t.Run("role_binding", func(t *testing.T) {
		cli := authzSessionClient(t, adminSessionID)
		var roleBindingID string
		var roleID string

		// Create a role for assigning to user.
		bindingRoleName := authzTestUsername("rb_role")
		role, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
			Name: bindingRoleName,
		})
		require.NoError(t, err)
		require.NotEmpty(t, role.ID)
		roleID = role.ID

		t.Run("list", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.RoleBinding]](cli, roleBindingPath)
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
		})

		t.Run("create", func(t *testing.T) {
			createReq := &authz.RoleBinding{
				SubjectID: userID,
				RoleID:    roleID,
			}
			rsp, err := client.Post[authz.RoleBinding](cli, roleBindingPath, createReq)
			require.NoError(t, err)
			require.NotEmpty(t, rsp.ID)
			require.EqualValues(t, tenant.Default, rsp.TenantID)
			require.Equal(t, userID, rsp.SubjectID)
			require.Equal(t, roleID, rsp.RoleID)
			roleBindingID = rsp.ID
			requireCasbinGroupingPolicy(t, userID, roleID, tenant.Default)
		})

		t.Run("get", func(t *testing.T) {
			rsp, err := client.Get[authz.RoleBinding](cli, roleBindingPath+"/"+roleBindingID)
			require.NoError(t, err)
			require.Equal(t, roleBindingID, rsp.ID)
			require.EqualValues(t, tenant.Default, rsp.TenantID)
			require.Equal(t, userID, rsp.SubjectID)
			require.Equal(t, roleID, rsp.RoleID)
		})

		t.Run("list_expand", func(t *testing.T) {
			list, err := client.Get[client.ListResult[*authz.RoleBinding]](cli, roleBindingPath)
			require.NoError(t, err)
			require.NotNil(t, list.Items)
			require.GreaterOrEqual(t, list.Total, 0)
		})

		// The binding rows and the rule that authorized them are cleared by two
		// different steps of one delete, and the rows going away is no evidence
		// that the authorization did: a rule left behind keeps allowing requests
		// with no record left to revoke it.
		t.Run("delete_role_cleans_bindings_and_their_rules", func(t *testing.T) {
			deletedRole, err := client.Post[authz.Role](cli, rolePath, &authz.Role{
				Name: "deleted_role",
			})
			require.NoError(t, err)
			require.NotEmpty(t, deletedRole.ID)
			deletedRoleID := deletedRole.ID

			binding, err := client.Post[authz.RoleBinding](cli, roleBindingPath, &authz.RoleBinding{
				SubjectID: userID,
				RoleID:    deletedRoleID,
			})
			require.NoError(t, err)
			require.NotEmpty(t, binding.ID)

			_, err = client.Delete[struct{}](cli, rolePath+"/"+deletedRoleID, nil)
			require.NoError(t, err)

			remaining := make([]*authz.RoleBinding, 0)
			err = database.Database[*authz.RoleBinding](context.Background()).
				WithQuery(&authz.RoleBinding{Scope: tenant.Scope{TenantID: tenant.Default}, RoleID: deletedRoleID}).
				List(&remaining)
			require.NoError(t, err)
			require.Empty(t, remaining)

			require.Empty(t, storedPolicies(t, "g", userID, deletedRoleID),
				"deleting a role must take the assignments that reached it with it")
		})

		t.Run("delete", func(t *testing.T) {
			_, err := client.Delete[struct{}](cli, roleBindingPath+"/"+roleBindingID, nil)
			require.NoError(t, err, "delete should return success")
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
	authzGrantTenantPolicy(t, tenantA, adminRoleID,
		types.Permission{Object: "/api/iam/admin/users/{id}/status", Action: http.MethodPatch})
	tenantAMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_iam_member_a_role"))
	authzBindTenantRole(t, tenantA, targetTenantAUserID, tenantAMemberRoleID)
	tenantBMemberRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_iam_member_b_role"))
	authzBindTenantRole(t, tenantB, targetTenantBUserID, tenantBMemberRoleID)
	rootMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_iam_root_member_role"))
	authzBindTenantRole(t, tenantA, rootUsername, rootMemberRoleID)

	cli := authzTenantClient(t, adminSessionID, tenantA)
	rsp, err := client.Patch[iam.UserStatusPatchRsp](cli, userAdminPath+"/"+targetTenantAUserID+"/status",
		iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.NoError(t, err)
	require.NotEmpty(t, rsp.Msg)

	_, err = client.Patch[iam.UserStatusPatchRsp](cli, userAdminPath+"/"+rootUsername+"/status",
		iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	testutil.RequireError(t, err, http.StatusForbidden)

	_, err = client.Patch[iam.UserStatusPatchRsp](cli, userAdminPath+"/"+targetTenantBUserID+"/status",
		iam.UserStatusPatchReq{Status: iam.UserStatusActive})
	require.Error(t, err)

	cli = authzTenantClient(t, adminSessionID, tenantB)
	_, err = client.Patch[iam.UserStatusPatchRsp](cli, userAdminPath+"/"+targetTenantAUserID+"/status",
		iam.UserStatusPatchReq{Status: iam.UserStatusActive})
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
	authzGrantTenantPolicy(t, tenantA, adminRoleID,
		types.Permission{Object: "/api/iam/admin/users", Action: http.MethodGet},
		types.Permission{Object: "/api/iam/admin/users/{id}", Action: http.MethodGet})
	tenantAMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_admin_users_member_a_role"))
	authzBindTenantRole(t, tenantA, targetTenantAUserID, tenantAMemberRoleID)
	tenantBMemberRoleID := authzCreateTenantRole(t, tenantB, authzTestUsername("tenant_admin_users_member_b_role"))
	authzBindTenantRole(t, tenantB, targetTenantBUserID, tenantBMemberRoleID)
	rootMemberRoleID := authzCreateTenantRole(t, tenantA, authzTestUsername("tenant_admin_users_root_member_role"))
	authzBindTenantRole(t, tenantA, rootUsername, rootMemberRoleID)

	cli := authzTenantClient(t, adminSessionID, tenantA)

	t.Run("list_tenant_users", func(t *testing.T) {
		list, err := client.Get[client.ListResult[iam.AdminUserView]](cli, userAdminPath)
		require.NoError(t, err)
		require.Positive(t, list.Total)
		requireAdminUserView(t, list.Items, adminUserID)
		requireAdminUserView(t, list.Items, targetTenantAUserID)
		requireNoAdminUserView(t, list.Items, targetTenantBUserID)
		requireNoAdminUserView(t, list.Items, rootUsername)
	})

	t.Run("get_tenant_user", func(t *testing.T) {
		got, err := client.Get[iam.AdminUserGetRsp](cli, userAdminPath+"/"+targetTenantAUserID)
		require.NoError(t, err)
		require.Equal(t, targetTenantAUserID, got.User.ID)
	})

	t.Run("get_other_tenant_user_forbidden", func(t *testing.T) {
		_, err := client.Get[iam.AdminUserGetRsp](cli, userAdminPath+"/"+targetTenantBUserID)
		require.Error(t, err)
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

	session, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), modeliamsession.SessionIDKey(sessionID))
	require.NoError(t, err)
	require.Equal(t, tenantID, session.TenantID)
}

func TestIAMLoginRejectsTenantOutsideMembership(t *testing.T) {
	username := authzTestUsername("tenant_login_forbidden_user")
	password := "12345678"
	authzSignupUser(t, username, password)

	cli, err := client.New(baseURL, client.WithUserAgent(tenantUserAgent))
	require.NoError(t, err)

	_, err = client.Post[iam.LoginRsp](cli, loginPath, iam.LoginReq{
		Username: username,
		Password: password,
		TenantID: authzTestUsername("tenant_login_forbidden"),
	})
	testutil.RequireError(t, err, http.StatusForbidden)
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
	cli, err := client.New(baseURL, options...)
	require.NoError(t, err)

	apiResp, err := cli.Do(http.MethodPost, loginPath, reqPayload)
	require.NoError(t, err)

	rsp := testutil.DecodeResp[iam.LoginRsp](t, apiResp)
	require.False(t, rsp.ServerTime.IsZero())
	require.False(t, rsp.Session.ExpiresAt.IsZero())
	if reqPayload.TenantID != "" {
		require.Equal(t, reqPayload.TenantID, rsp.Session.TenantID)
	}

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	cookie := apiResp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	require.NotEmpty(t, cookie.Value)
	require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
	return cookie.Value
}

// authzSessionClient returns a client that presents the given session id.
func authzSessionClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

// authzTenantClient returns a client that presents the given session id and
// resolves into the given tenant through the tenant header.
func authzTenantClient(t *testing.T, sessionID, tenantID string) *client.Client {
	t.Helper()

	cli, err := client.New(
		baseURL,
		client.WithHeader(http.Header{
			tenantHeader: []string{tenantID},
		}),
		client.WithUserAgent(tenantUserAgent),
		client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}),
	)
	require.NoError(t, err)
	return cli
}

func authzCreateTenantRole(t *testing.T, tenantID, name string, menuIDs ...string) string {
	t.Helper()

	// The tenant comes from the context, which is what a request would supply;
	// setting the column here would be overwritten by the stamp anyway.
	ctx := tenant.In(context.Background(), tenantID)
	role := &authz.Role{
		Base:    model.Base{ID: util.HashID(tenantID, name)},
		Name:    name,
		MenuIDs: menuIDs,
	}
	require.NoError(t, database.Database[*authz.Role](ctx).Create(role))
	return role.ID
}

func authzBindTenantRole(t *testing.T, tenantID, subjectID, roleID string) {
	t.Helper()

	ctx := tenant.In(context.Background(), tenantID)
	roleBinding := &authz.RoleBinding{
		SubjectID: subjectID,
		RoleID:    roleID,
	}
	require.NoError(t, database.Database[*authz.RoleBinding](ctx).Create(roleBinding))
}

// authzGrantTenantPolicy sets the whole permission set of a role, because that
// is what the API takes: the argument is the truth about the role, so calling
// it twice replaces rather than accumulates.
func authzGrantTenantPolicy(t *testing.T, tenantID, roleID string, permissions ...types.Permission) {
	t.Helper()

	require.NoError(t, rbac.RBAC().SetRolePermissions(context.Background(), tenantID, roleID, permissions))
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
	requireAuthzRule(t, "p", tenant, role, object, action, effect)
}

func requireNoCasbinPolicy(t *testing.T, tenant, role, object, action, effect string) {
	t.Helper()
	requireNoAuthzRule(t, "p", tenant, role, object, action, effect)
}

func requireCasbinGroupingPolicy(t *testing.T, subject, role, tenant string) {
	t.Helper()
	requireAuthzRule(t, "g", subject, role, tenant, "", "")
}

func requireAuthzRule(t *testing.T, ptype, v0, v1, v2, v3, v4 string) {
	t.Helper()
	rules := listAuthzRules(t, ptype, v0, v1, v2, v3, v4)
	require.NotEmpty(t, rules)
}

func requireNoAuthzRule(t *testing.T, ptype, v0, v1, v2, v3, v4 string) {
	t.Helper()
	rules := listAuthzRules(t, ptype, v0, v1, v2, v3, v4)
	require.Empty(t, rules)
}

func listAuthzRules(t *testing.T, ptype, v0, v1, v2, v3, v4 string) []*authz.AuthzRule {
	t.Helper()
	rules := make([]*authz.AuthzRule, 0)
	require.NoError(t, database.Database[*authz.AuthzRule](context.Background()).WithQuery(&authz.AuthzRule{
		Ptype: ptype,
		V0:    v0,
		V1:    v1,
		V2:    v2,
		V3:    v3,
		V4:    v4,
	}).List(&rules))
	return rules
}
