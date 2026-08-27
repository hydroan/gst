package database_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

func TestDatabaseCleanup(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Verify initial count - should have 3 records
	count := new(int)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 3, *count)

	// Soft delete some records (u1 and u2)
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1, u2))

	// Verify soft-deleted records are not visible in normal queries
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 1, *count, "only u3 should be visible after soft delete")

	// Verify u3 is still accessible
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u3.ID))
	require.NotNil(t, u)
	require.Equal(t, u3.ID, u.ID)
	require.Equal(t, u3.Name, u.Name)

	// Cleanup permanently deletes the soft-deleted records (u1 and u2)
	require.NoError(t, database.Cleanup[*TestUser](context.Background()))

	// Verify soft-deleted records are permanently removed
	// After Cleanup, u1 and u2 should be permanently deleted
	// u3 should still exist
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 1, *count, "u3 should still exist after Cleanup")

	// Verify u3 is still accessible
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u3.ID))
	require.NotNil(t, u)
	require.Equal(t, u3.ID, u.ID)
	require.Equal(t, u3.Name, u.Name)
	require.Equal(t, u3.Age, u.Age)
	require.Equal(t, u3.Email, u.Email)

	// Cleanup with no soft-deleted records left - should not error
	require.NoError(t, database.Cleanup[*TestUser](context.Background()))

	// Verify u3 still exists after second Cleanup
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 1, *count, "u3 should still exist after second Cleanup")

	// Cleanup works for different model types
	require.NoError(t, database.Cleanup[*TestItem](context.Background()))
	require.NoError(t, database.Cleanup[*TestCategory](context.Background()))
}

func TestDatabaseCleanupOn(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// CleanupOn against the default handle behaves exactly like Cleanup.
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	require.NoError(t, database.CleanupOn[*TestUser](context.Background(), database.DB()))

	count := new(int)
	require.NoError(t, database.Database[*TestUser](context.Background()).WithDeleted().Count(count))
	require.Equal(t, 2, *count, "CleanupOn should remove the soft-deleted row for good")
}

func TestDatabaseHealth(t *testing.T) {
	// Basic health check - should pass when the database is healthy, and stay
	// idempotent across repeated probes.
	require.NoError(t, database.Health(context.Background()))
	require.NoError(t, database.Health(context.Background()))

	// Health check after database operations - should still pass
	defer cleanupTestData()
	setupTestData(t)
	require.NoError(t, database.Health(context.Background()))
}

func TestDatabaseHealthOn(t *testing.T) {
	// HealthOn against the default handle behaves exactly like Health.
	require.NoError(t, database.HealthOn(context.Background(), database.DB()))

	// An open transaction is not a connection pool; HealthOn refuses it.
	tx := database.DB().Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.ErrorIs(t, database.HealthOn(context.Background(), tx), database.ErrTransactionInstance)
}
