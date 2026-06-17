package iam_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/helper"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/response"
	"github.com/stretchr/testify/require"
)

type tenantBatchRsp struct {
	Items   []*iam.Tenant `json:"items"`
	Summary struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"summary"`
}

func TestTenantCreate(t *testing.T) {
	actor := userSignupUser(t, "tenant_create_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := tenantNewClient(t, actor.SessionID)
	tenantName := fmt.Sprintf("tenant_create_target_%d", time.Now().UnixNano())

	t.Cleanup(func() {
		tenantCleanupByName(t, tenantName)
	})

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Create(iam.Tenant{Name: tenantName})
		userRequireForbidden(t, err)
	})

	t.Run("create_tenant_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Create(iam.Tenant{Name: tenantName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Tenant) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			require.Equal(t, tenantName, rsp.Name)
		})

		stored := tenantLoadByName(t, tenantName)
		require.Equal(t, tenantName, stored.Name)
	})
}

func TestTenantGet(t *testing.T) {
	actor := userSignupUser(t, "tenant_get_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenantName := fmt.Sprintf("tenant_get_target_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenantName)
	})

	target := tenantLoadByName(t, tenantName)
	cli := tenantNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Get(target.ID, new(iam.Tenant))
		userRequireForbidden(t, err)
	})

	t.Run("get_tenant_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		got := new(iam.Tenant)
		resp, err := cli.Get(target.ID, got)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)
		require.Equal(t, target.ID, got.ID)
		require.Equal(t, tenantName, got.Name)
	})

	t.Run("get_tenant_not_found", func(t *testing.T) {
		_, err := cli.Get("missing-tenant-id", new(iam.Tenant))
		userRequireNotFound(t, err)
	})
}

func TestTenantList(t *testing.T) {
	actor := userSignupUser(t, "tenant_list_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenantName := fmt.Sprintf("tenant_list_target_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenantName)
	})

	cli := tenantNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		items := make([]*iam.Tenant, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		userRequireForbidden(t, err)
	})

	t.Run("list_tenants_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		items := make([]*iam.Tenant, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*iam.Tenant]) {
			t.Helper()
			require.GreaterOrEqual(t, rsp.Total, int64(1))
			require.NotNil(t, tenantFindByName(rsp.Items, tenantName))
		})
	})
}

func TestTenantUpdate(t *testing.T) {
	actor := userSignupUser(t, "tenant_update_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenantName := fmt.Sprintf("tenant_update_target_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenantName)
		tenantCleanupByName(t, tenantName+"_updated")
	})

	target := tenantLoadByName(t, tenantName)
	cli := tenantNewClient(t, actor.SessionID)
	updatedName := tenantName + "_updated"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Update(target.ID, iam.Tenant{Name: updatedName})
		userRequireForbidden(t, err)
	})

	t.Run("update_tenant_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Update(target.ID, iam.Tenant{Name: updatedName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Tenant) {
			t.Helper()
			require.Equal(t, target.ID, rsp.ID)
			require.Equal(t, updatedName, rsp.Name)
		})

		stored := tenantLoadByName(t, updatedName)
		require.Equal(t, target.ID, stored.ID)
	})

	t.Run("update_tenant_not_found", func(t *testing.T) {
		_, err := cli.Update("missing-tenant-id", iam.Tenant{Name: "missing_tenant"})
		userRequireNotFound(t, err)
	})
}

func TestTenantCreateMany(t *testing.T) {
	actor := userSignupUser(t, "tenant_create_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := tenantNewClient(t, actor.SessionID)
	name1 := fmt.Sprintf("tenant_create_many_1_%d", time.Now().UnixNano())
	name2 := fmt.Sprintf("tenant_create_many_2_%d", time.Now().UnixNano())

	t.Cleanup(func() {
		tenantCleanupByName(t, name1)
		tenantCleanupByName(t, name2)
	})

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.CreateMany([]iam.Tenant{
			{Name: name1},
			{Name: name2},
		})
		userRequireForbidden(t, err)
	})

	t.Run("create_many_tenants_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.CreateMany([]iam.Tenant{
			{Name: name1},
			{Name: name2},
		})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp tenantBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)

			item1 := tenantFindByName(rsp.Items, name1)
			require.NotNil(t, item1)
			require.NotEmpty(t, item1.ID)

			item2 := tenantFindByName(rsp.Items, name2)
			require.NotNil(t, item2)
			require.NotEmpty(t, item2.ID)
		})
	})

	tenant1 := tenantLoadByName(t, name1)
	require.Equal(t, name1, tenant1.Name)

	tenant2 := tenantLoadByName(t, name2)
	require.Equal(t, name2, tenant2.Name)
}

func TestTenantPatch(t *testing.T) {
	actor := userSignupUser(t, "tenant_patch_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenantName := fmt.Sprintf("tenant_patch_target_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenantName)
		tenantCleanupByName(t, tenantName+"_patched")
	})

	target := tenantLoadByName(t, tenantName)
	cli := tenantNewClient(t, actor.SessionID)
	patchedName := tenantName + "_patched"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Patch(target.ID, iam.Tenant{Name: patchedName})
		userRequireForbidden(t, err)
	})

	t.Run("patch_tenant_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Patch(target.ID, iam.Tenant{Name: patchedName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Tenant) {
			t.Helper()
			require.Equal(t, target.ID, rsp.ID)
			require.Equal(t, patchedName, rsp.Name)
		})

		stored := tenantLoadByName(t, patchedName)
		require.Equal(t, target.ID, stored.ID)
	})

	t.Run("patch_tenant_not_found", func(t *testing.T) {
		_, err := cli.Patch("missing-tenant-id", iam.Tenant{Name: "missing_tenant"})
		userRequireNotFound(t, err)
	})
}

func TestTenantUpdateMany(t *testing.T) {
	actor := userSignupUser(t, "tenant_update_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenant1Name := fmt.Sprintf("tenant_update_many_source_1_%d", time.Now().UnixNano())
	tenant2Name := fmt.Sprintf("tenant_update_many_source_2_%d", time.Now().UnixNano())
	tenantCreate(t, tenant1Name)
	tenantCreate(t, tenant2Name)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenant1Name)
		tenantCleanupByName(t, tenant2Name)
		tenantCleanupByName(t, tenant1Name+"_updated")
		tenantCleanupByName(t, tenant2Name+"_updated")
	})

	cli := tenantNewClient(t, actor.SessionID)
	tenant1 := tenantLoadByName(t, tenant1Name)
	tenant2 := tenantLoadByName(t, tenant2Name)
	tenant1.Name = tenant1Name + "_updated"
	tenant2.Name = tenant2Name + "_updated"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.UpdateMany([]*iam.Tenant{tenant1, tenant2})
		userRequireForbidden(t, err)
	})

	t.Run("update_many_tenants_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.UpdateMany([]*iam.Tenant{tenant1, tenant2})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp tenantBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)
		})

		updated1 := tenantLoadByName(t, tenant1.Name)
		require.Equal(t, tenant1.Name, updated1.Name)

		updated2 := tenantLoadByName(t, tenant2.Name)
		require.Equal(t, tenant2.Name, updated2.Name)
	})
}

func TestTenantPatchMany(t *testing.T) {
	actor := userSignupUser(t, "tenant_patch_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenant1Name := fmt.Sprintf("tenant_patch_many_source_1_%d", time.Now().UnixNano())
	tenant2Name := fmt.Sprintf("tenant_patch_many_source_2_%d", time.Now().UnixNano())
	tenantCreate(t, tenant1Name)
	tenantCreate(t, tenant2Name)

	t.Cleanup(func() {
		tenantCleanupByName(t, tenant1Name)
		tenantCleanupByName(t, tenant2Name)
		tenantCleanupByName(t, tenant1Name+"_patched")
		tenantCleanupByName(t, tenant2Name+"_patched")
	})

	cli := tenantNewClient(t, actor.SessionID)
	tenant1 := &iam.Tenant{Base: tenantLoadByName(t, tenant1Name).Base, Name: tenant1Name + "_patched"}
	tenant2 := &iam.Tenant{Base: tenantLoadByName(t, tenant2Name).Base, Name: tenant2Name + "_patched"}

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.PatchMany([]*iam.Tenant{tenant1, tenant2})
		userRequireForbidden(t, err)
	})

	t.Run("patch_many_tenants_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.PatchMany([]*iam.Tenant{tenant1, tenant2})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp tenantBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)
		})

		patched1 := tenantLoadByName(t, tenant1.Name)
		require.Equal(t, tenant1.Name, patched1.Name)

		patched2 := tenantLoadByName(t, tenant2.Name)
		require.Equal(t, tenant2.Name, patched2.Name)
	})
}

func TestTenantDelete(t *testing.T) {
	actor := userSignupUser(t, "tenant_delete_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenantName := fmt.Sprintf("tenant_delete_target_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)
	target := tenantLoadByName(t, tenantName)
	cli := tenantNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Delete(target.ID)
		userRequireForbidden(t, err)
	})

	t.Run("delete_tenant_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Delete(target.ID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		tenantRequireMissingByName(t, tenantName)
	})
}

func TestTenantDeleteMany(t *testing.T) {
	actor := userSignupUser(t, "tenant_delete_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	tenant1Name := fmt.Sprintf("tenant_delete_many_source_1_%d", time.Now().UnixNano())
	tenant2Name := fmt.Sprintf("tenant_delete_many_source_2_%d", time.Now().UnixNano())
	tenantCreate(t, tenant1Name)
	tenantCreate(t, tenant2Name)

	cli := tenantNewClient(t, actor.SessionID)
	tenant1 := tenantLoadByName(t, tenant1Name)
	tenant2 := tenantLoadByName(t, tenant2Name)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.DeleteMany([]string{tenant1.ID, tenant2.ID})
		userRequireForbidden(t, err)
	})

	t.Run("delete_many_tenants_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.DeleteMany([]string{tenant1.ID, tenant2.ID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		tenantRequireMissingByName(t, tenant1Name)
		tenantRequireMissingByName(t, tenant2Name)
	})
}

func tenantNewClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(tenantAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func tenantCreate(t *testing.T, name string) {
	t.Helper()

	tenant := &iam.Tenant{Name: name}
	require.NoError(t, database.Database[*iam.Tenant](nil).Create(tenant))
}

func tenantCleanupByName(t *testing.T, name string) {
	t.Helper()

	tenants := make([]*iam.Tenant, 0)
	require.NoError(t, database.Database[*iam.Tenant](nil).WithQuery(&iam.Tenant{Name: name}).List(&tenants))
	if len(tenants) == 0 {
		return
	}

	require.NoError(t, database.Database[*iam.Tenant](nil).Delete(tenants...))
}

func tenantLoadByName(t *testing.T, name string) *iam.Tenant {
	t.Helper()

	tenants := make([]*iam.Tenant, 0)
	require.NoError(t, database.Database[*iam.Tenant](nil).WithLimit(1).WithQuery(&iam.Tenant{Name: name}).List(&tenants))
	require.Len(t, tenants, 1)
	require.NotNil(t, tenants[0])
	require.NotEmpty(t, tenants[0].ID)
	return tenants[0]
}

func tenantRequireMissingByName(t *testing.T, name string) {
	t.Helper()

	tenants := make([]*iam.Tenant, 0)
	require.NoError(t, database.Database[*iam.Tenant](nil).WithQuery(&iam.Tenant{Name: name}).List(&tenants))
	require.Empty(t, tenants)
}

func tenantFindByName(items []*iam.Tenant, name string) *iam.Tenant {
	for _, item := range items {
		if item != nil && item.Name == name {
			return item
		}
	}
	return nil
}
