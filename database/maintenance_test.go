package database_test

import (
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

func TestDatabaseCleanup(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Verify initial count - should have 3 records
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](nil).Count(count))
	require.Equal(t, int64(3), *count)

	// Soft delete some records (u1 and u2)
	require.NoError(t, database.Database[*TestUser](nil).Delete(u1, u2))

	// Verify soft-deleted records are not visible in normal queries
	require.NoError(t, database.Database[*TestUser](nil).Count(count))
	require.Equal(t, int64(1), *count, "only u3 should be visible after soft delete")

	// Verify u3 is still accessible
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).Get(u, u3.ID))
	require.NotNil(t, u)
	require.Equal(t, u3.ID, u.ID)
	require.Equal(t, u3.Name, u.Name)

	// Test Cleanup - should permanently delete soft-deleted records (u1 and u2)
	require.NoError(t, database.Database[*TestUser](nil).Cleanup())

	// Verify soft-deleted records are permanently removed
	// After Cleanup, u1 and u2 should be permanently deleted
	// u3 should still exist
	require.NoError(t, database.Database[*TestUser](nil).Count(count))
	require.Equal(t, int64(1), *count, "u3 should still exist after Cleanup")

	// Verify u3 is still accessible
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).Get(u, u3.ID))
	require.NotNil(t, u)
	require.Equal(t, u3.ID, u.ID)
	require.Equal(t, u3.Name, u.Name)
	require.Equal(t, u3.Age, u.Age)
	require.Equal(t, u3.Email, u.Email)

	// Test Cleanup with no soft-deleted records - should not error
	require.NoError(t, database.Database[*TestUser](nil).Cleanup())

	// Verify u3 still exists after second Cleanup
	require.NoError(t, database.Database[*TestUser](nil).Count(count))
	require.Equal(t, int64(1), *count, "u3 should still exist after second Cleanup")

	// Test Cleanup with different model types
	require.NoError(t, database.Database[*TestProduct](nil).Cleanup())
	require.NoError(t, database.Database[*TestCategory](nil).Cleanup())
}

func TestDatabaseCleanupWithDryRun(t *testing.T) {
	require.NoError(t, database.DB().AutoMigrate(&cleanupSoftDeleteUser{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&cleanupSoftDeleteUser{}))
	})

	u1 := &cleanupSoftDeleteUser{Name: "cleanup-user-1", Base: model.Base{ID: "cleanup-user-1"}}
	u2 := &cleanupSoftDeleteUser{Name: "cleanup-user-2", Base: model.Base{ID: "cleanup-user-2"}}
	require.NoError(t, database.Database[*cleanupSoftDeleteUser](nil).Create(u1, u2))
	require.NoError(t, database.Database[*cleanupSoftDeleteUser](nil).Delete(u1, u2))
	require.Equal(t, int64(2), countSoftDeletedCleanupUsers(t), "setup should leave two soft-deleted users")

	require.NoError(t, database.Database[*cleanupSoftDeleteUser](nil).WithDryRun().Cleanup())
	require.Equal(t, int64(2), countSoftDeletedCleanupUsers(t), "dry-run Cleanup should not remove soft-deleted users")

	require.NoError(t, database.Database[*cleanupSoftDeleteUser](nil).Cleanup())
	require.Equal(t, int64(0), countSoftDeletedCleanupUsers(t), "Cleanup should permanently remove soft-deleted users")
}

func TestDatabaseHealth(t *testing.T) {
	// Test basic health check - should pass when database is healthy
	require.NoError(t, database.Database[*TestUser](nil).Health())

	// Test health check multiple times - should be idempotent
	require.NoError(t, database.Database[*TestUser](nil).Health())
	require.NoError(t, database.Database[*TestUser](nil).Health())

	// Test health check after database operations - should still pass
	defer cleanupTestData()
	setupTestData(t)
	require.NoError(t, database.Database[*TestUser](nil).Health())

	// Test health check with different model types - should work for all models
	require.NoError(t, database.Database[*TestProduct](nil).Health())
	require.NoError(t, database.Database[*TestCategory](nil).Health())
}

type cleanupSoftDeleteUser struct {
	Name string `json:"name"`

	model.Base
}

func countSoftDeletedCleanupUsers(t *testing.T) int64 {
	t.Helper()

	var count int64
	require.NoError(t, database.DB().Model(&cleanupSoftDeleteUser{}).Unscoped().Where("deleted_at IS NOT NULL").Count(&count).Error)
	return count
}
