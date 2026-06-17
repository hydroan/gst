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

type groupBatchRsp struct {
	Items   []*iam.Group `json:"items"`
	Summary struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"summary"`
}

func TestGroupCreate(t *testing.T) {
	actor := userSignupUser(t, "group_create_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := groupNewClient(t, actor.SessionID)
	groupName := fmt.Sprintf("group_create_target_%d", time.Now().UnixNano())

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
	})

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Create(iam.Group{Name: groupName})
		userRequireForbidden(t, err)
	})

	t.Run("create_group_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Create(iam.Group{Name: groupName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Group) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			require.Equal(t, groupName, rsp.Name)
		})

		stored := groupLoadByName(t, groupName)
		require.Equal(t, groupName, stored.Name)
	})
}

func TestGroupCreateWithTenant(t *testing.T) {
	actor := userSignupUser(t, "group_create_tenant_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)
	userSetSuperuser(t, actor.Username, true)

	tenantName := fmt.Sprintf("group_create_tenant_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)
	tenant := tenantLoadByName(t, tenantName)
	cli := groupNewClient(t, actor.SessionID)

	groupName := fmt.Sprintf("group_create_with_tenant_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		tenantCleanupByName(t, tenantName)
	})

	t.Run("create_group_with_missing_tenant", func(t *testing.T) {
		missingTenantID := "missing-tenant-id"
		_, err := cli.Create(iam.Group{Name: groupName, TenantID: &missingTenantID})
		userRequireNotFound(t, err)
	})

	t.Run("create_group_with_existing_tenant", func(t *testing.T) {
		resp, err := cli.Create(iam.Group{Name: groupName, TenantID: &tenant.ID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		stored := groupLoadByName(t, groupName)
		require.NotNil(t, stored.TenantID)
		require.Equal(t, tenant.ID, *stored.TenantID)
	})
}

func TestGroupGet(t *testing.T) {
	actor := userSignupUser(t, "group_get_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	groupName := fmt.Sprintf("group_get_target_%d", time.Now().UnixNano())
	groupCreate(t, groupName)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
	})

	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Get(target.ID, new(iam.Group))
		userRequireForbidden(t, err)
	})

	t.Run("get_group_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		got := new(iam.Group)
		resp, err := cli.Get(target.ID, got)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)
		require.Equal(t, target.ID, got.ID)
		require.Equal(t, groupName, got.Name)
	})

	t.Run("get_group_not_found", func(t *testing.T) {
		_, err := cli.Get("missing-group-id", new(iam.Group))
		userRequireNotFound(t, err)
	})
}

func TestGroupList(t *testing.T) {
	actor := userSignupUser(t, "group_list_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	groupName := fmt.Sprintf("group_list_target_%d", time.Now().UnixNano())
	groupCreate(t, groupName)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
	})

	cli := groupNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		items := make([]*iam.Group, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		userRequireForbidden(t, err)
	})

	t.Run("list_groups_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		items := make([]*iam.Group, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*iam.Group]) {
			t.Helper()
			require.GreaterOrEqual(t, rsp.Total, int64(1))
			require.NotNil(t, groupFindByName(rsp.Items, groupName))
		})
	})
}

func TestGroupUpdate(t *testing.T) {
	actor := userSignupUser(t, "group_update_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	groupName := fmt.Sprintf("group_update_target_%d", time.Now().UnixNano())
	groupCreate(t, groupName)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		groupCleanupByName(t, groupName+"_updated")
	})

	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)
	updatedName := groupName + "_updated"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Update(target.ID, iam.Group{Name: updatedName})
		userRequireForbidden(t, err)
	})

	t.Run("update_group_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Update(target.ID, iam.Group{Name: updatedName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Group) {
			t.Helper()
			require.Equal(t, target.ID, rsp.ID)
			require.Equal(t, updatedName, rsp.Name)
		})

		stored := groupLoadByName(t, updatedName)
		require.Equal(t, target.ID, stored.ID)
	})

	t.Run("update_group_not_found", func(t *testing.T) {
		_, err := cli.Update("missing-group-id", iam.Group{Name: "missing_group"})
		userRequireNotFound(t, err)
	})
}

func TestGroupUpdateWithTenant(t *testing.T) {
	actor := userSignupUser(t, "group_update_tenant_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)
	userSetSuperuser(t, actor.Username, true)

	tenantName := fmt.Sprintf("group_update_tenant_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)
	tenant := tenantLoadByName(t, tenantName)

	groupName := fmt.Sprintf("group_update_with_tenant_%d", time.Now().UnixNano())
	groupCreate(t, groupName)
	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		tenantCleanupByName(t, tenantName)
	})

	t.Run("update_group_with_missing_tenant", func(t *testing.T) {
		missingTenantID := "missing-tenant-id"
		_, err := cli.Update(target.ID, iam.Group{Name: groupName, TenantID: &missingTenantID})
		userRequireNotFound(t, err)
	})

	t.Run("update_group_with_existing_tenant", func(t *testing.T) {
		resp, err := cli.Update(target.ID, iam.Group{Name: groupName, TenantID: &tenant.ID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		stored := groupLoadByName(t, groupName)
		require.NotNil(t, stored.TenantID)
		require.Equal(t, tenant.ID, *stored.TenantID)
	})
}

func TestGroupCreateMany(t *testing.T) {
	actor := userSignupUser(t, "group_create_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := groupNewClient(t, actor.SessionID)
	name1 := fmt.Sprintf("group_create_many_1_%d", time.Now().UnixNano())
	name2 := fmt.Sprintf("group_create_many_2_%d", time.Now().UnixNano())

	t.Cleanup(func() {
		groupCleanupByName(t, name1)
		groupCleanupByName(t, name2)
	})

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.CreateMany([]iam.Group{
			{Name: name1},
			{Name: name2},
		})
		userRequireForbidden(t, err)
	})

	t.Run("create_many_groups_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.CreateMany([]iam.Group{
			{Name: name1},
			{Name: name2},
		})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp groupBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)

			item1 := groupFindByName(rsp.Items, name1)
			require.NotNil(t, item1)
			require.NotEmpty(t, item1.ID)

			item2 := groupFindByName(rsp.Items, name2)
			require.NotNil(t, item2)
			require.NotEmpty(t, item2.ID)
		})
	})

	group1 := groupLoadByName(t, name1)
	require.Equal(t, name1, group1.Name)

	group2 := groupLoadByName(t, name2)
	require.Equal(t, name2, group2.Name)
}

func TestGroupCreateManyWithTenant(t *testing.T) {
	actor := userSignupUser(t, "group_create_many_tenant_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)
	userSetSuperuser(t, actor.Username, true)

	tenantName := fmt.Sprintf("group_create_many_tenant_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)
	tenant := tenantLoadByName(t, tenantName)
	cli := groupNewClient(t, actor.SessionID)

	groupName := fmt.Sprintf("group_create_many_with_tenant_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		tenantCleanupByName(t, tenantName)
	})

	t.Run("create_many_groups_with_missing_tenant", func(t *testing.T) {
		missingTenantID := "missing-tenant-id"
		_, err := cli.CreateMany([]iam.Group{{Name: groupName, TenantID: &missingTenantID}})
		userRequireNotFound(t, err)
	})

	t.Run("create_many_groups_with_existing_tenant", func(t *testing.T) {
		resp, err := cli.CreateMany([]iam.Group{{Name: groupName, TenantID: &tenant.ID}})
		require.NoError(t, err)
		require.NotNil(t, resp)

		stored := groupLoadByName(t, groupName)
		require.NotNil(t, stored.TenantID)
		require.Equal(t, tenant.ID, *stored.TenantID)
	})
}

func TestGroupUpdateMany(t *testing.T) {
	actor := userSignupUser(t, "group_update_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	group1Name := fmt.Sprintf("group_update_many_source_1_%d", time.Now().UnixNano())
	group2Name := fmt.Sprintf("group_update_many_source_2_%d", time.Now().UnixNano())
	groupCreate(t, group1Name)
	groupCreate(t, group2Name)

	t.Cleanup(func() {
		groupCleanupByName(t, group1Name)
		groupCleanupByName(t, group2Name)
		groupCleanupByName(t, group1Name+"_updated")
		groupCleanupByName(t, group2Name+"_updated")
	})

	cli := groupNewClient(t, actor.SessionID)
	group1 := groupLoadByName(t, group1Name)
	group2 := groupLoadByName(t, group2Name)
	group1.Name = group1Name + "_updated"
	group2.Name = group2Name + "_updated"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.UpdateMany([]*iam.Group{group1, group2})
		userRequireForbidden(t, err)
	})

	t.Run("update_many_groups_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.UpdateMany([]*iam.Group{group1, group2})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp groupBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)
		})

		updated1 := groupLoadByName(t, group1.Name)
		require.Equal(t, group1.Name, updated1.Name)

		updated2 := groupLoadByName(t, group2.Name)
		require.Equal(t, group2.Name, updated2.Name)
	})
}

func TestGroupPatchWithTenant(t *testing.T) {
	actor := userSignupUser(t, "group_patch_tenant_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)
	userSetSuperuser(t, actor.Username, true)

	tenantName := fmt.Sprintf("group_patch_tenant_%d", time.Now().UnixNano())
	tenantCreate(t, tenantName)
	tenant := tenantLoadByName(t, tenantName)

	groupName := fmt.Sprintf("group_patch_with_tenant_%d", time.Now().UnixNano())
	groupCreate(t, groupName)
	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		tenantCleanupByName(t, tenantName)
	})

	t.Run("patch_group_with_missing_tenant", func(t *testing.T) {
		missingTenantID := "missing-tenant-id"
		_, err := cli.Patch(target.ID, iam.Group{TenantID: &missingTenantID})
		userRequireNotFound(t, err)
	})

	t.Run("patch_group_with_existing_tenant", func(t *testing.T) {
		resp, err := cli.Patch(target.ID, iam.Group{TenantID: &tenant.ID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		stored := groupLoadByName(t, groupName)
		require.NotNil(t, stored.TenantID)
		require.Equal(t, tenant.ID, *stored.TenantID)
	})
}

func TestGroupPatch(t *testing.T) {
	actor := userSignupUser(t, "group_patch_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	groupName := fmt.Sprintf("group_patch_target_%d", time.Now().UnixNano())
	groupCreate(t, groupName)

	t.Cleanup(func() {
		groupCleanupByName(t, groupName)
		groupCleanupByName(t, groupName+"_patched")
	})

	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)
	patchedName := groupName + "_patched"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Patch(target.ID, iam.Group{Name: patchedName})
		userRequireForbidden(t, err)
	})

	t.Run("patch_group_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Patch(target.ID, iam.Group{Name: patchedName})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.Group) {
			t.Helper()
			require.Equal(t, target.ID, rsp.ID)
			require.Equal(t, patchedName, rsp.Name)
		})

		stored := groupLoadByName(t, patchedName)
		require.Equal(t, target.ID, stored.ID)
	})

	t.Run("patch_group_not_found", func(t *testing.T) {
		_, err := cli.Patch("missing-group-id", iam.Group{Name: "missing_group"})
		userRequireNotFound(t, err)
	})
}

func TestGroupPatchMany(t *testing.T) {
	actor := userSignupUser(t, "group_patch_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	group1Name := fmt.Sprintf("group_patch_many_source_1_%d", time.Now().UnixNano())
	group2Name := fmt.Sprintf("group_patch_many_source_2_%d", time.Now().UnixNano())
	groupCreate(t, group1Name)
	groupCreate(t, group2Name)

	t.Cleanup(func() {
		groupCleanupByName(t, group1Name)
		groupCleanupByName(t, group2Name)
		groupCleanupByName(t, group1Name+"_patched")
		groupCleanupByName(t, group2Name+"_patched")
	})

	cli := groupNewClient(t, actor.SessionID)
	group1 := &iam.Group{Base: groupLoadByName(t, group1Name).Base, Name: group1Name + "_patched"}
	group2 := &iam.Group{Base: groupLoadByName(t, group2Name).Base, Name: group2Name + "_patched"}

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.PatchMany([]*iam.Group{group1, group2})
		userRequireForbidden(t, err)
	})

	t.Run("patch_many_groups_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.PatchMany([]*iam.Group{group1, group2})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp groupBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)
		})

		patched1 := groupLoadByName(t, group1.Name)
		require.Equal(t, group1.Name, patched1.Name)

		patched2 := groupLoadByName(t, group2.Name)
		require.Equal(t, group2.Name, patched2.Name)
	})
}

func TestGroupDelete(t *testing.T) {
	actor := userSignupUser(t, "group_delete_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	groupName := fmt.Sprintf("group_delete_target_%d", time.Now().UnixNano())
	groupCreate(t, groupName)
	target := groupLoadByName(t, groupName)
	cli := groupNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Delete(target.ID)
		userRequireForbidden(t, err)
	})

	t.Run("delete_group_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.Delete(target.ID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		groupRequireMissingByName(t, groupName)
	})
}

func TestGroupDeleteMany(t *testing.T) {
	actor := userSignupUser(t, "group_delete_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	group1Name := fmt.Sprintf("group_delete_many_source_1_%d", time.Now().UnixNano())
	group2Name := fmt.Sprintf("group_delete_many_source_2_%d", time.Now().UnixNano())
	groupCreate(t, group1Name)
	groupCreate(t, group2Name)

	cli := groupNewClient(t, actor.SessionID)
	group1 := groupLoadByName(t, group1Name)
	group2 := groupLoadByName(t, group2Name)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.DeleteMany([]string{group1.ID, group2.ID})
		userRequireForbidden(t, err)
	})

	t.Run("delete_many_groups_after_promote_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)

		resp, err := cli.DeleteMany([]string{group1.ID, group2.ID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, response.CodeSuccess.Code(), resp.Code)

		groupRequireMissingByName(t, group1Name)
		groupRequireMissingByName(t, group2Name)
	})
}

func groupNewClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(groupAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func groupCreate(t *testing.T, name string) {
	t.Helper()

	group := &iam.Group{Name: name}
	require.NoError(t, database.Database[*iam.Group](nil).Create(group))
}

func groupCleanupByName(t *testing.T, name string) {
	t.Helper()

	groups := make([]*iam.Group, 0)
	require.NoError(t, database.Database[*iam.Group](nil).WithQuery(&iam.Group{Name: name}).List(&groups))
	if len(groups) == 0 {
		return
	}

	require.NoError(t, database.Database[*iam.Group](nil).Delete(groups...))
}

func groupLoadByName(t *testing.T, name string) *iam.Group {
	t.Helper()

	groups := make([]*iam.Group, 0)
	require.NoError(t, database.Database[*iam.Group](nil).WithLimit(1).WithQuery(&iam.Group{Name: name}).List(&groups))
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0])
	require.NotEmpty(t, groups[0].ID)
	return groups[0]
}

func groupRequireMissingByName(t *testing.T, name string) {
	t.Helper()

	groups := make([]*iam.Group, 0)
	require.NoError(t, database.Database[*iam.Group](nil).WithQuery(&iam.Group{Name: name}).List(&groups))
	require.Empty(t, groups)
}

func groupFindByName(items []*iam.Group, name string) *iam.Group {
	for _, item := range items {
		if item != nil && item.Name == name {
			return item
		}
	}
	return nil
}
