package database_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDatabaseWithIndex(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	existsIndex := "idx_test_users_created_by" // index auto created when database migration.
	notExistsIndex := "not_exists_index"

	users := make([]*TestUser, 0)

	// Test WithIndex with default hint (USE INDEX)
	// Note: Index hints only work on MySQL. On SQLite/PostgreSQL, it will skip silently.
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).List(&users))
	require.Len(t, users, 3)
	// Verify returned data integrity
	var foundU1, foundU2, foundU3 bool
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
			require.NotEmpty(t, u.ID)
			require.NotEmpty(t, u.CreatedAt)
			require.NotEmpty(t, u.UpdatedAt)
			require.Equal(t, u1.Name, u.Name)
			require.Equal(t, u1.Age, u.Age)
			require.Equal(t, u1.Email, u.Email)
		case u2.ID:
			foundU2 = true
			require.NotEmpty(t, u.ID)
			require.NotEmpty(t, u.CreatedAt)
			require.NotEmpty(t, u.UpdatedAt)
			require.Equal(t, u2.Name, u.Name)
			require.Equal(t, u2.Age, u.Age)
			require.Equal(t, u2.Email, u.Email)
		case u3.ID:
			foundU3 = true
			require.NotEmpty(t, u.ID)
			require.NotEmpty(t, u.CreatedAt)
			require.NotEmpty(t, u.UpdatedAt)
			require.Equal(t, u3.Name, u.Name)
			require.Equal(t, u3.Age, u.Age)
			require.Equal(t, u3.Email, u.Email)
		}
	}
	require.True(t, foundU1, "should find u1")
	require.True(t, foundU2, "should find u2")
	require.True(t, foundU3, "should find u3")

	// Test WithIndex with explicit USE hint
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex, consts.IndexHintUse).List(&users))
	require.Len(t, users, 3)

	// Test WithIndex with FORCE hint
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex, consts.IndexHintForce).List(&users))
	require.Len(t, users, 3)

	// Test WithIndex with IGNORE hint
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex, consts.IndexHintIgnore).List(&users))
	require.Len(t, users, 3)

	// Test WithIndex with empty index name (should be ignored)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex("").List(&users))
	require.Len(t, users, 3, "empty index name should be ignored and query should work normally")

	// Test WithIndex with whitespace-only index name (should be ignored)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex("   ").List(&users))
	require.Len(t, users, 3, "whitespace-only index name should be ignored and query should work normally")

	// Test WithIndex combined with WithQuery
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).
		WithIndex(existsIndex).
		WithQuery(&TestUser{Name: u1.Name}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)
	require.Equal(t, u1.Name, users[0].Name)

	// Test WithIndex with non-existent index
	// Note: On MySQL, this will cause an error. On SQLite/PostgreSQL, it will skip silently.
	// The behavior depends on the database type, so we test that it doesn't panic.
	users = make([]*TestUser, 0)
	// On SQLite, this will skip the index hint and work normally
	// On MySQL, this might cause an error depending on the index existence
	err := database.Database[*TestUser](nil).WithIndex(notExistsIndex).List(&users)
	// We don't assert error here because behavior differs by database type
	_ = err

	// Test WithIndex with empty result set
	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).List(&users))
	require.Empty(t, users, "should return empty result when no records exist")

	// Test WithIndex with Get method (single record)
	require.NoError(t, database.Database[*TestUser](nil).Create(ul...))
	user := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).Get(user, u1.ID))
	require.NotNil(t, user)
	require.Equal(t, u1.ID, user.ID)
	require.Equal(t, u1.Name, user.Name)
	require.Equal(t, u1.Age, user.Age)
	require.Equal(t, u1.Email, user.Email)

	// Test WithIndex with Count method
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).Count(count))
	require.Equal(t, int64(3), *count)

	// Test WithIndex with First method
	firstUser := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).First(firstUser))
	require.NotNil(t, firstUser)
	require.NotEmpty(t, firstUser.ID)

	// Test WithIndex with Last method
	lastUser := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).WithIndex(existsIndex).Last(lastUser))
	require.NotNil(t, lastUser)
	require.NotEmpty(t, lastUser.ID)
}

func TestDatabaseWithCursor(t *testing.T) {
	defer cleanupTestData()

	t.Run("NextPage", func(t *testing.T) {
		defer cleanupTestData()
		count := 100
		data := make([]*TestUser, 0, count)
		for i := range count {
			name := fmt.Sprintf("user%05d", i)
			data = append(data, &TestUser{Name: name, Base: model.Base{ID: name}})
		}
		require.NoError(t, database.Database[*TestUser](nil).WithBatchSize(1000).Create(data...))

		// Get first record as starting cursor
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).First(u))
		cursorValue := u.ID
		require.Equal(t, "user00000", cursorValue, "first record should be user00000")

		// Test pagination: fetch next pages
		users := make([]*TestUser, 0)
		for i := range 10 {
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).
				WithLimit(1).
				WithCursor(cursorValue, true).
				List(&users))
			require.Len(t, users, 1, "should return 1 record per page")
			expectedID := fmt.Sprintf("user%05d", i+1)
			require.Equal(t, expectedID, users[0].ID, "should fetch next record in ascending order")
			cursorValue = users[0].ID
		}
	})

	t.Run("PreviousPage", func(t *testing.T) {
		defer cleanupTestData()
		count := 100
		data := make([]*TestUser, 0, count)
		for i := range count {
			name := fmt.Sprintf("user%05d", i)
			data = append(data, &TestUser{Name: name, Base: model.Base{ID: name}})
		}
		require.NoError(t, database.Database[*TestUser](nil).WithBatchSize(1000).Create(data...))

		// Get last record as starting cursor
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Last(u))
		cursorValue := u.ID
		require.Equal(t, fmt.Sprintf("user%05d", count-1), cursorValue, "last record should be user00099")

		// Test pagination: fetch previous pages
		users := make([]*TestUser, 0)
		for i := range 10 {
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).
				WithLimit(1).
				WithCursor(cursorValue, false).
				List(&users))
			require.Len(t, users, 1, "should return 1 record per page")
			expectedID := fmt.Sprintf("user%05d", count-2-i)
			require.Equal(t, expectedID, users[0].ID, "should fetch previous record in descending order")
			cursorValue = users[0].ID
		}
	})

	t.Run("CustomField", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test cursor pagination with custom field (created_at)
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).List(&users))
		require.Len(t, users, 3)

		// Get first record's created_at as cursor
		// Format time to match database format (YYYY-MM-DD HH:MM:SS.ffffff)
		firstUser := users[0]
		require.NotNil(t, firstUser.CreatedAt, "first user should have created_at")
		cursorValue := firstUser.CreatedAt.Format("2006-01-02 15:04:05.000000")

		// Fetch next page using created_at as cursor field
		nextUsers := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithLimit(1).
			WithCursor(cursorValue, true, "created_at").
			List(&nextUsers))
		if len(nextUsers) > 0 {
			require.NotEqual(t, firstUser.ID, nextUsers[0].ID, "should fetch different record when available")
			require.NotNil(t, nextUsers[0].CreatedAt, "next user should have created_at")
			require.True(t, nextUsers[0].CreatedAt.After(*firstUser.CreatedAt) ||
				nextUsers[0].CreatedAt.Equal(*firstUser.CreatedAt),
				"next record should have created_at >= cursor value")
		}
	})

	t.Run("EmptyCursor", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test with empty cursor value (should be ignored)
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithLimit(10).
			WithCursor("", true).
			List(&users))
		require.Len(t, users, 3, "empty cursor should be ignored, return all records")
	})

	t.Run("Combined", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test cursor pagination combined with WithQuery
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name}).
			List(&users))
		require.Len(t, users, 1)

		cursorValue := users[0].ID
		nextUsers := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name}).
			WithLimit(1).
			WithCursor(cursorValue, true).
			List(&nextUsers))
		require.Empty(t, nextUsers, "no more records after cursor with query condition")
	})

	t.Run("MultiplePages", func(t *testing.T) {
		defer cleanupTestData()
		count := 50
		data := make([]*TestUser, 0, count)
		for i := range count {
			name := fmt.Sprintf("user%05d", i)
			data = append(data, &TestUser{Name: name, Base: model.Base{ID: name}})
		}
		require.NoError(t, database.Database[*TestUser](nil).WithBatchSize(1000).Create(data...))

		// Test pagination with page size > 1
		pageSize := 10
		cursorValue := ""
		allFetched := make([]string, 0)

		for range 5 {
			users := make([]*TestUser, 0)
			db := database.Database[*TestUser](nil).WithLimit(pageSize)
			if cursorValue != "" {
				db = db.WithCursor(cursorValue, true)
			}
			require.NoError(t, db.List(&users))
			require.LessOrEqual(t, len(users), pageSize, "should not exceed page size")

			if len(users) == 0 {
				break
			}

			for _, u := range users {
				allFetched = append(allFetched, u.ID)
			}
			cursorValue = users[len(users)-1].ID
		}

		require.NotEmpty(t, allFetched, "should fetch at least some records")
		// Verify no duplicates
		seen := make(map[string]bool)
		for _, id := range allFetched {
			require.False(t, seen[id], "should not have duplicate records: %s", id)
			seen[id] = true
		}
	})
}

func TestDatabaseWithSelect(t *testing.T) {
	defer cleanupTestData()

	// No effect on "Create"
	t.Run("Create", func(t *testing.T) {
		t.Run("with existing column", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name").Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
		})
		t.Run("with non-existing column", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("notexists").Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
		})
	})

	// No effect on "Delete"
	t.Run("Delete", func(t *testing.T) {
		t.Run("with existing column", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name").Create(ul...))
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name").Delete(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Empty(t, users)
		})
		t.Run("with non-existing column", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("notexists").Create(ul...))
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("notexists").Delete(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Empty(t, users)
		})
	})

	// Effect "Update"
	t.Run("Update", func(t *testing.T) {
		t.Run("with existing column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			u1.Name = "user1_modified"
			u2.Name = "user2_modified"
			u3.Name = "user3_modified"
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name").Update(ul...))

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, "user1_modified", u11.Name)
			require.Equal(t, "user2_modified", u22.Name)
			require.Equal(t, "user3_modified", u33.Name)
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			require.Equal(t, u1.Email, u11.Email)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u3.Email, u33.Email)
			require.Equal(t, u1.IsActive, u11.IsActive)
			require.Equal(t, u2.IsActive, u22.IsActive)
			require.Equal(t, u3.IsActive, u33.IsActive)
		})
		t.Run("with different column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			u1OldName, u2OldName, u3OldName := u1.Name, u2.Name, u3.Name
			u1.Name = "user1_modified"
			u2.Name = "user2_modified"
			u3.Name = "user3_modified"
			// Only update column "age", the modified name will not be updated.
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("age").Update(ul...))

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, u1OldName, u11.Name, "name should not be updated")
			require.Equal(t, u2OldName, u22.Name, "name should not be updated")
			require.Equal(t, u3OldName, u33.Name, "name should not be updated")
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			require.Equal(t, u1.Email, u11.Email)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u3.Email, u33.Email)
			require.Equal(t, u1.IsActive, u11.IsActive)
			require.Equal(t, u2.IsActive, u22.IsActive)
			require.Equal(t, u3.IsActive, u33.IsActive)
		})
		t.Run("with non-existing column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			u1OldName, u2OldName, u3OldName := u1.Name, u2.Name, u3.Name
			u1.Name = "user1_modified"
			u2.Name = "user2_modified"
			u3.Name = "user3_modified"
			// The non-existing fields will be ignored, and only default columns will be selected.
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("nonexistent").Update(ul...))

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, u1OldName, u11.Name, "name should not be updated")
			require.Equal(t, u2OldName, u22.Name, "name should not be updated")
			require.Equal(t, u3OldName, u33.Name, "name should not be updated")
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			require.Equal(t, u1.Email, u11.Email)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u3.Email, u33.Email)
			require.Equal(t, u1.IsActive, u11.IsActive)
			require.Equal(t, u2.IsActive, u22.IsActive)
			require.Equal(t, u3.IsActive, u33.IsActive)
		})
		t.Run("with multiple columns", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			u1.Name = "user1_modified"
			u1.Age = 25
			u2.Name = "user2_modified"
			u2.Age = 26
			u3.Name = "user3_modified"
			u3.Age = 27
			// Update both "name" and "age" columns.
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name", "age").Update(ul...))

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, "user1_modified", u11.Name)
			require.Equal(t, "user2_modified", u22.Name)
			require.Equal(t, "user3_modified", u33.Name)
			require.Equal(t, 25, u11.Age)
			require.Equal(t, 26, u22.Age)
			require.Equal(t, 27, u33.Age)
			require.Equal(t, u1.Email, u11.Email, "email should not be updated")
			require.Equal(t, u2.Email, u22.Email, "email should not be updated")
			require.Equal(t, u3.Email, u33.Email, "email should not be updated")
		})
	})

	// Effect "List"
	t.Run("List", func(t *testing.T) {
		t.Run("with single column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			// Only select column "name", other columns will be ignored.
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name").List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, u1.Name, u11.Name)
			require.Equal(t, u2.Name, u22.Name)
			require.Equal(t, u3.Name, u33.Name)
			// Only select "name", fields "age" and "email" should be empty.
			require.Empty(t, u11.Age)
			require.Empty(t, u22.Age)
			require.Empty(t, u33.Age)
			require.Empty(t, u11.Email)
			require.Empty(t, u22.Email)
			require.Empty(t, u33.Email)
		})
		t.Run("with different column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			// Only select column "age", other columns will be ignored.
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("age").List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Empty(t, u11.Name)
			require.Empty(t, u22.Name)
			require.Empty(t, u33.Name)
			// Only select "age", fields "name" and "email" should be empty.
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			require.Empty(t, u11.Email)
			require.Empty(t, u22.Email)
			require.Empty(t, u33.Email)
		})
		t.Run("with multiple columns", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			// Select both "name" and "age" columns.
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithSelect("name", "age").List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			require.Equal(t, u1.Name, u11.Name)
			require.Equal(t, u2.Name, u22.Name)
			require.Equal(t, u3.Name, u33.Name)
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			// Field "email" should be empty.
			require.Empty(t, u11.Email)
			require.Empty(t, u22.Email)
			require.Empty(t, u33.Email)
		})
		t.Run("with non-existing column", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			// Selecting non-existing column will cause error.
			users := make([]*TestUser, 0)
			require.Error(t, database.Database[*TestUser](nil).WithSelect("notexists").List(&users))
		})
		t.Run("with empty columns", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			// WithSelect with no columns is a no-op, so all columns should be selected (default behavior).
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithSelect().List(&users))
			require.Len(t, users, 3)
			u11, u22, u33 := findUsersByID(users)
			require.NotNil(t, u11)
			require.NotNil(t, u22)
			require.NotNil(t, u33)
			// All columns should be present since WithSelect() with no args is a no-op.
			require.NotEmpty(t, u11.ID)
			require.NotEmpty(t, u22.ID)
			require.NotEmpty(t, u33.ID)
			require.NotEmpty(t, u11.CreatedAt)
			require.NotEmpty(t, u22.CreatedAt)
			require.NotEmpty(t, u33.CreatedAt)
			// Verify that other fields are also present (not just default columns)
			require.NotEmpty(t, u11.Name)
			require.NotEmpty(t, u22.Name)
			require.NotEmpty(t, u33.Name)
		})
	})
}

func TestDatabaseWithOrder(t *testing.T) {
	assertNameOrder := func(t *testing.T, users []*TestUser, expected []string) {
		t.Helper()
		require.Len(t, users, len(expected))
		for i := range expected {
			require.Equal(t, expected[i], users[i].Name)
		}
	}
	assertIDOrder := func(t *testing.T, users []*TestUser, expected []string) {
		t.Helper()
		require.Len(t, users, len(expected))
		for i := range expected {
			require.Equal(t, expected[i], users[i].ID)
		}
	}

	t.Run("SingleField", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("name").List(&users))
		assertNameOrder(t, users, []string{u1.Name, u2.Name, u3.Name})

		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("name asc").List(&users))
		assertNameOrder(t, users, []string{u1.Name, u2.Name, u3.Name})

		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("name desc").List(&users))
		assertNameOrder(t, users, []string{u3.Name, u2.Name, u1.Name})

		// WithOrder should also affect First/Last style queries.
		user := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("name desc").First(user))
		require.Equal(t, u3.ID, user.ID)
	})

	t.Run("MultipleFields", func(t *testing.T) {
		t.Run("two-level sort", func(t *testing.T) {
			defer cleanupTestData()
			u1.Age = 30
			u2.Age = 30
			u3.Age = 18
			u1.Name = "anna"
			u2.Name = "beth"
			u3.Name = "charlie"
			setupTestData(t)

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithOrder("age desc, name asc").List(&users))
			assertNameOrder(t, users, []string{u1.Name, u2.Name, u3.Name})
		})

		t.Run("three-level sort", func(t *testing.T) {
			defer cleanupTestData()
			u1.Age = 30
			u2.Age = 30
			u3.Age = 25
			u1.Name = "alex"
			u2.Name = "alex"
			u3.Name = "zack"
			u1.Email = "alex1@example.com"
			u2.Email = "alex2@example.com"
			u3.Email = "zack@example.com"
			setupTestData(t)

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithOrder("age DESC, name asc, email desc").List(&users))
			assertIDOrder(t, users, []string{u2.ID, u1.ID, u3.ID})
		})
	})

	t.Run("InvalidOrder", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.Error(t, database.Database[*TestUser](nil).WithOrder("name invalid_direction").List(&users))
	})
}

func TestDatabaseWithPagination(t *testing.T) {
	defer cleanupTestData()

	newSeqUsers := func(prefix string, count int) ([]*TestUser, []string) {
		users := make([]*TestUser, 0, count)
		ids := make([]string, 0, count)
		for i := range count {
			id := fmt.Sprintf("%s_%05d", prefix, i)
			users = append(users, &TestUser{
				Name:  fmt.Sprintf("%s_name_%05d", prefix, i),
				Email: fmt.Sprintf("%s_%05d@example.com", prefix, i),
				Age:   18 + i,
				Base:  model.Base{ID: id},
			})
			ids = append(ids, id)
		}
		return users, ids
	}
	assertIDs := func(t *testing.T, users []*TestUser, expected []string) {
		t.Helper()
		require.Len(t, users, len(expected))
		for i := range expected {
			require.Equal(t, expected[i], users[i].ID)
		}
	}
	runPage := func(t *testing.T, page, size int) []*TestUser {
		t.Helper()
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithPagination(page, size).
			List(&users))
		return users
	}

	t.Run("BasicPaging", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_basic", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		assertIDs(t, runPage(t, 1, 3), ids[:3])
		assertIDs(t, runPage(t, 2, 3), ids[3:6])
		assertIDs(t, runPage(t, 4, 3), ids[9:])
	})

	t.Run("PageOutOfRange", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_out_of_range", 5)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		assertIDs(t, runPage(t, 2, 3), ids[3:])
		assertIDs(t, runPage(t, 3, 3), []string{})
	})

	t.Run("NonPositivePageDefaultsToOne", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_non_positive", 6)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		assertIDs(t, runPage(t, 0, 3), ids[:3])
		assertIDs(t, runPage(t, -5, 3), ids[:3])
	})

	t.Run("NonPositiveSizeUsesDefaultLimit", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_size", 7)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		assertIDs(t, runPage(t, 1, 0), ids)
		assertIDs(t, runPage(t, 1, -10), ids)
	})

	t.Run("LargePageSize", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_large_size", 8)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		assertIDs(t, runPage(t, 1, 50), ids)
		assertIDs(t, runPage(t, 2, 50), []string{})
	})

	t.Run("EquivalentToOffsetAndLimit", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("pg_offset", 9)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		pageUsers := runPage(t, 3, 2)
		offsetUsers := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithOffset(4).
			WithLimit(2).
			List(&offsetUsers))

		assertIDs(t, pageUsers, ids[4:6])
		assertIDs(t, offsetUsers, ids[4:6])
	})
}

func TestDatabaseWithOffset(t *testing.T) {
	defer cleanupTestData()

	newSeqUsers := func(prefix string, count int) ([]*TestUser, []string) {
		users := make([]*TestUser, 0, count)
		ids := make([]string, 0, count)
		for i := range count {
			id := fmt.Sprintf("%s_%05d", prefix, i)
			users = append(users, &TestUser{
				Name:  fmt.Sprintf("%s_name_%05d", prefix, i),
				Email: fmt.Sprintf("%s_%05d@example.com", prefix, i),
				Age:   18 + i,
				Base:  model.Base{ID: id},
			})
			ids = append(ids, id)
		}
		return users, ids
	}
	assertIDs := func(t *testing.T, users []*TestUser, expected []string) {
		t.Helper()
		require.Len(t, users, len(expected))
		for i := range expected {
			require.Equal(t, expected[i], users[i].ID)
		}
	}

	t.Run("BasicOffset", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("offset_basic", 8)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithOffset(2).
			WithLimit(3).
			List(&users))

		assertIDs(t, users, ids[2:5])
	})

	t.Run("NonPositiveOffsetDoesNotSkip", func(t *testing.T) {
		defer cleanupTestData()
		records, ids := newSeqUsers("offset_non_positive", 5)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithOffset(0).
			WithLimit(3).
			List(&users))
		assertIDs(t, users, ids[:3])

		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithOffset(-10).
			WithLimit(3).
			List(&users))
		assertIDs(t, users, ids[:3])
	})

	t.Run("ChainOrder", func(t *testing.T) {
		defer cleanupTestData()
		records, _ := newSeqUsers("offset_chain", 8)
		require.NoError(t, database.Database[*TestUser](nil).Create(records...))

		users1 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithOffset(3).
			WithLimit(2).
			List(&users1))

		users2 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithOrder("id asc").
			WithLimit(2).
			WithOffset(3).
			List(&users2))

		require.Equal(t, users1, users2)
	})
}

func TestDatabaseWithLimit(t *testing.T) {
	defer cleanupTestData()

	newSeqUsers := func(prefix string, count int) ([]*TestUser, []string) {
		users := make([]*TestUser, 0, count)
		ids := make([]string, 0, count)
		for i := range count {
			id := fmt.Sprintf("%s_%05d", prefix, i)
			users = append(users, &TestUser{
				Name:  fmt.Sprintf("%s_name_%05d", prefix, i),
				Email: fmt.Sprintf("%s_%05d@example.com", prefix, i),
				Age:   18 + i,
				Base:  model.Base{ID: id},
			})
			ids = append(ids, id)
		}
		return users, ids
	}
	assertIDs := func(t *testing.T, users []*TestUser, expected []string) {
		t.Helper()
		require.Len(t, users, len(expected))
		for i := range expected {
			require.Equal(t, expected[i], users[i].ID)
		}
	}

	t.Run("BasicLimit", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(3).List(&users))
		require.Len(t, users, 3)
		assertIDs(t, users, testIDs[0:3])
	})

	t.Run("WithOrderAsc", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(5).List(&users))
		require.Len(t, users, 5)
		assertIDs(t, users, testIDs[0:5])
	})

	t.Run("WithOrderDesc", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id desc").WithLimit(5).List(&users))
		require.Len(t, users, 5)
		// Reverse expected IDs for desc order
		expected := make([]string, 5)
		for i := range 5 {
			expected[i] = testIDs[9-i]
		}
		assertIDs(t, users, expected)
	})

	t.Run("LimitExceedsTotal", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 5)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(10).List(&users))
		require.Len(t, users, 5)
		assertIDs(t, users, testIDs)
	})

	t.Run("NonPositiveLimitUsesDefault", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		// limit <= 0 should use defaultLimit (-1, unlimited)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(0).List(&users))
		require.Len(t, users, 10)
		assertIDs(t, users, testIDs)

		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(-1).List(&users))
		require.Len(t, users, 10)
		assertIDs(t, users, testIDs)

		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(-99).List(&users))
		require.Len(t, users, 10)
		assertIDs(t, users, testIDs)
	})

	t.Run("LimitOne", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, testIDs := newSeqUsers("test", 5)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id asc").WithLimit(1).List(&users))
		require.Len(t, users, 1)
		assertIDs(t, users, testIDs[0:1])
	})

	t.Run("ChainOrder", func(t *testing.T) {
		defer cleanupTestData()
		testUsers, _ := newSeqUsers("test", 10)
		require.NoError(t, database.Database[*TestUser](nil).Create(testUsers...))

		// Test that WithOrder can be chained before or after WithLimit
		users1 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithOrder("id desc").WithLimit(3).List(&users1))

		users2 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithLimit(3).WithOrder("id desc").List(&users2))

		// Both should produce the same result
		require.Len(t, users2, len(users1))
		require.Equal(t, users1[0].ID, users2[0].ID)
		require.Equal(t, users1[1].ID, users2[1].ID)
		require.Equal(t, users1[2].ID, users2[2].ID)
	})
}

func TestDatabaseWithExpand(t *testing.T) {
	defer cleanupTestData()

	setupCategoryData := func(t *testing.T) {
		t.Helper()
		require.NoError(t, database.Database[*TestCategory](nil).Create(categoryRoot))
		require.NoError(t, database.Database[*TestCategory](nil).Create(categoryParent))

		children := []*TestCategory{
			{
				Name:     "child1",
				ParentID: categoryParentID,
				Base:     model.Base{ID: "child1"},
			},
			{
				Name:     "child2",
				ParentID: categoryParentID,
				Base:     model.Base{ID: "child2"},
			},
		}
		require.NoError(t, database.Database[*TestCategory](nil).Create(children...))
	}

	t.Run("Parent", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		categories := make([]*TestCategory, 0)
		// Expand parent with single depth
		require.NoError(t, database.Database[*TestCategory](nil).
			WithQuery(&TestCategory{Name: "child"}, types.QueryConfig{FuzzyMatch: true}).
			WithExpand([]string{"Parent"}).
			List(&categories))

		require.Len(t, categories, 2)
		require.NotNil(t, categories[0])
		require.NotNil(t, categories[1])
		require.NotNil(t, categories[0].Parent)
		require.NotNil(t, categories[1].Parent)
		require.Equal(t, categoryParentID, categories[0].Parent.ID)
		require.Equal(t, categoryParentID, categories[1].Parent.ID)

		// Only expand 1 depth, Parent.Parent should be nil
		require.Nil(t, categories[0].Parent.Parent)
		require.Nil(t, categories[1].Parent.Parent)
	})

	t.Run("ParentWithTwoDepth", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		categories := make([]*TestCategory, 0)
		// Expand parent with two depth (Parent.Parent)
		require.NoError(t, database.Database[*TestCategory](nil).
			WithQuery(&TestCategory{Name: "child"}, types.QueryConfig{FuzzyMatch: true}).
			WithExpand([]string{"Parent.Parent"}).
			List(&categories))

		require.Len(t, categories, 2)
		require.NotNil(t, categories[0])
		require.NotNil(t, categories[1])
		require.NotNil(t, categories[0].Parent)
		require.NotNil(t, categories[1].Parent)
		require.Equal(t, categoryParentID, categories[0].Parent.ID)
		require.Equal(t, categoryParentID, categories[1].Parent.ID)

		// Expand root (Parent.Parent)
		require.NotNil(t, categories[0].Parent.Parent)
		require.NotNil(t, categories[1].Parent.Parent)
		require.Equal(t, categoryRootID, categories[0].Parent.Parent.ID)
		require.Equal(t, categoryRootID, categories[1].Parent.Parent.ID)
	})

	t.Run("ParentWithMoreDepth", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		categories := make([]*TestCategory, 0)
		// Expand with more depth than available (should only expand available depth)
		require.NoError(t, database.Database[*TestCategory](nil).
			WithQuery(&TestCategory{Name: "child"}, types.QueryConfig{FuzzyMatch: true}).
			WithExpand([]string{"Parent.Parent.Parent.Parent"}).
			List(&categories))

		require.Len(t, categories, 2)
		require.NotNil(t, categories[0])
		require.NotNil(t, categories[1])
		require.NotNil(t, categories[0].Parent)
		require.NotNil(t, categories[1].Parent)
		require.Equal(t, categoryParentID, categories[0].Parent.ID)
		require.Equal(t, categoryParentID, categories[1].Parent.ID)

		// Should only expand available depth (2 levels: parent -> root)
		require.NotNil(t, categories[0].Parent.Parent)
		require.NotNil(t, categories[1].Parent.Parent)
		require.Equal(t, categoryRootID, categories[0].Parent.Parent.ID)
		require.Equal(t, categoryRootID, categories[1].Parent.Parent.ID)
	})

	t.Run("ParentCaseSensitive", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		categories := make([]*TestCategory, 0)
		// Association names are case sensitive
		require.Error(t, database.Database[*TestCategory](nil).
			WithQuery(&TestCategory{Name: "child"}, types.QueryConfig{FuzzyMatch: true}).
			WithExpand([]string{"parent"}).
			List(&categories))
	})

	t.Run("Children", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		c := new(TestCategory)
		require.NoError(t, database.Database[*TestCategory](nil).WithExpand([]string{"Children"}).Get(c, categoryRootID))
		require.Len(t, c.Children, 2)
		// Root has two children: itself and "parent"
		require.NotNil(t, c.Children[0])
		var r, p *TestCategory
		var foundR, foundP bool
		for _, child := range c.Children {
			switch child.ID {
			case categoryRootID:
				foundR = true
				r = child
			case categoryParentID:
				foundP = true
				p = child
			}
		}
		require.True(t, foundR)
		require.True(t, foundP)
		require.NotNil(t, r)
		require.NotNil(t, p)
		require.Equal(t, categoryRootID, r.ID)
		require.Equal(t, categoryParentID, p.ID)
		// Only one depth, no children
		require.Nil(t, r.Children)
		require.Nil(t, p.Children)
	})

	t.Run("ChildrenWithTwoDepth", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		c := new(TestCategory)
		require.NoError(t, database.Database[*TestCategory](nil).Get(c, categoryRootID))
		require.Empty(t, c.Children)

		require.NoError(t, database.Database[*TestCategory](nil).WithExpand([]string{"Children.Children"}).Get(c, categoryRootID))
		require.Len(t, c.Children, 2)
		// Root has two children: itself and "parent"
		require.NotNil(t, c.Children[0])
		var root, parent *TestCategory
		var foundR, foundP bool
		for _, child := range c.Children {
			switch child.ID {
			case categoryRootID:
				foundR = true
				root = child
			case categoryParentID:
				foundP = true
				parent = child
			}
		}
		require.True(t, foundR)
		require.True(t, foundP)
		require.NotNil(t, root)
		require.NotNil(t, parent)
		require.Equal(t, categoryRootID, root.ID)
		require.Equal(t, categoryParentID, parent.ID)

		// With two depth
		require.NotNil(t, root.Children)
		require.NotNil(t, parent.Children)
		require.Len(t, root.Children, 2)   // Root has two children: itself and "parent"
		require.Len(t, parent.Children, 2) // Parent has two children: "child1" and "child2"
		require.NotNil(t, root.Children[0])
		require.NotNil(t, root.Children[1])
		require.NotNil(t, parent.Children[0])
		require.NotNil(t, parent.Children[1])
	})

	t.Run("ChildrenWithMoreDepth", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		c := new(TestCategory)
		require.NoError(t, database.Database[*TestCategory](nil).Get(c, categoryRootID))
		require.Empty(t, c.Children)

		require.NoError(t, database.Database[*TestCategory](nil).WithExpand([]string{"Children.Children.Children.Children"}).Get(c, categoryRootID))
		require.Len(t, c.Children, 2)
		// Root has two children: itself and "parent"
		require.NotNil(t, c.Children[0])
		var root, parent *TestCategory
		var foundR, foundP bool
		for _, child := range c.Children {
			switch child.ID {
			case categoryRootID:
				foundR = true
				root = child
			case categoryParentID:
				foundP = true
				parent = child
			}
		}
		require.True(t, foundR)
		require.True(t, foundP)
		require.NotNil(t, root)
		require.NotNil(t, parent)
		require.Equal(t, categoryRootID, root.ID)
		require.Equal(t, categoryParentID, parent.ID)

		// Should only expand available depth (2 levels)
		require.NotNil(t, root.Children)
		require.NotNil(t, parent.Children)
		require.Len(t, root.Children, 2)   // Root has two children: itself and "parent"
		require.Len(t, parent.Children, 2) // Parent has two children: "child1" and "child2"
		require.NotNil(t, root.Children[0])
		require.NotNil(t, root.Children[1])
		require.NotNil(t, parent.Children[0])
		require.NotNil(t, parent.Children[1])
	})

	t.Run("ChildrenCaseSensitive", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		c := new(TestCategory)
		// Association names are case sensitive
		require.Error(t, database.Database[*TestCategory](nil).WithExpand([]string{"children"}).Get(c, categoryRootID))
	})

	t.Run("ParentAndChildren", func(t *testing.T) {
		defer cleanupTestData()
		setupCategoryData(t)

		c := new(TestCategory)
		require.NoError(t, database.Database[*TestCategory](nil).Get(c, categoryParentID))
		require.Empty(t, c.Children)
		require.Nil(t, c.Parent)

		require.NoError(t, database.Database[*TestCategory](nil).WithExpand([]string{"Parent", "Children"}).Get(c, categoryParentID))
		require.Len(t, c.Children, 2)
		require.NotNil(t, c.Children[0])
		require.NotNil(t, c.Children[1])
		require.NotNil(t, c.Parent)
		require.Equal(t, categoryRootID, c.Parent.ID)
	})
}

func TestDatabaseWithExclude(t *testing.T) {
	defer cleanupTestData()

	t.Run("ExcludeByID", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{
			"id": {u1.ID},
		}).List(&users))

		require.Len(t, users, 2)
		u11, u22, u33 := findUsersByID(users)
		require.Nil(t, u11, "u1 should be excluded")
		require.NotNil(t, u22)
		require.NotNil(t, u33)
		require.Equal(t, u2.ID, u22.ID)
		require.Equal(t, u3.ID, u33.ID)
	})

	t.Run("ExcludeByName", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{
			"name": {u2.Name},
		}).List(&users))

		require.Len(t, users, 2)
		u11, u22, u33 := findUsersByID(users)
		require.NotNil(t, u11)
		require.Nil(t, u22, "u2 should be excluded")
		require.NotNil(t, u33)
		require.Equal(t, u1.ID, u11.ID)
		require.Equal(t, u3.ID, u33.ID)
	})

	t.Run("ExcludeMultipleIDs", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{
			"id": {u1.ID, u2.ID},
		}).List(&users))

		require.Len(t, users, 1)
		u11, u22, u33 := findUsersByID(users)
		require.Nil(t, u11, "u1 should be excluded")
		require.Nil(t, u22, "u2 should be excluded")
		require.NotNil(t, u33)
		require.Equal(t, u3.ID, u33.ID)
	})

	t.Run("ExcludeMultipleFields", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{
			"id":   {u1.ID},
			"name": {u2.Name},
		}).List(&users))

		require.Len(t, users, 1)
		u11, u22, u33 := findUsersByID(users)
		require.Nil(t, u11, "u1 should be excluded by id")
		require.Nil(t, u22, "u2 should be excluded by name")
		require.NotNil(t, u33)
		require.Equal(t, u3.ID, u33.ID)
	})

	t.Run("ExcludeAll", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{
			"id": {u1.ID, u2.ID, u3.ID},
		}).List(&users))

		require.Empty(t, users, "all users should be excluded")
	})

	t.Run("ExcludeEmptyMap", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithExclude(map[string][]any{}).List(&users))

		require.Len(t, users, 3, "empty exclude map should not filter any records")
		u11, u22, u33 := findUsersByID(users)
		require.NotNil(t, u11)
		require.NotNil(t, u22)
		require.NotNil(t, u33)
	})
}

func TestDatabaseWithPurge(t *testing.T) {
	defer cleanupTestData()

	t.Run("HardDeleteDefault", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge() defaults to true (hard delete)
		require.NoError(t, database.Database[*TestUser](nil).WithPurge().Delete(u1))

		// Verify record is permanently deleted (not visible in normal queries)
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(2), *count, "should have 2 records after hard delete")

		// Verify record is not accessible via Get
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(u, u1.ID))
		require.Empty(t, u.ID, "hard-deleted record should not be accessible via Get")

		// Verify record is permanently deleted (not found even with Unscoped)
		var unscopedUser TestUser
		err := database.DB().Unscoped().Where("id = ?", u1.ID).First(&unscopedUser).Error
		require.Error(t, err, "hard-deleted record should not exist even with Unscoped")
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("HardDeleteExplicit", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge(true) explicitly enables hard delete
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).Delete(u1, u2))

		// Verify records are permanently deleted
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(1), *count, "should have 1 record after hard delete")

		// Verify records are not found even with Unscoped
		var unscopedUser TestUser
		err := database.DB().Unscoped().Where("id = ?", u1.ID).First(&unscopedUser).Error
		require.Error(t, err)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)

		err = database.DB().Unscoped().Where("id = ?", u2.ID).First(&unscopedUser).Error
		require.Error(t, err)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)

		// Verify u3 still exists
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(u, u3.ID))
		require.Equal(t, u3.ID, u.ID)
	})

	t.Run("SoftDeleteExplicit", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge(false) explicitly enables soft delete (overrides model.Purge())
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).Delete(u1))

		// Verify record is soft-deleted (not visible in normal queries)
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(2), *count, "should have 2 records after soft delete")

		// Verify record is not accessible via Get
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(u, u1.ID))
		require.Empty(t, u.ID, "soft-deleted record should not be accessible via Get")

		// Verify record still exists in database (found with Unscoped)
		var unscopedUser TestUser
		require.NoError(t, database.DB().Unscoped().Where("id = ?", u1.ID).First(&unscopedUser).Error)
		require.Equal(t, u1.ID, unscopedUser.ID, "soft-deleted record should exist with Unscoped")
		require.NotNil(t, unscopedUser.DeletedAt, "soft-deleted record should have deleted_at set")
	})

	t.Run("BatchHardDelete", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Batch hard delete all records
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).Delete(ul...))

		// Verify all records are permanently deleted
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(0), *count, "should have 0 records after batch hard delete")

		// Verify all records are not found even with Unscoped
		var countUnscoped int64
		require.NoError(t, database.DB().Unscoped().Model(&TestUser{}).Count(&countUnscoped).Error)
		require.Equal(t, int64(0), countUnscoped, "all records should be permanently deleted")
	})

	t.Run("BatchSoftDelete", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Batch soft delete all records
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).Delete(ul...))

		// Verify all records are soft-deleted (not visible in normal queries)
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(0), *count, "should have 0 records after batch soft delete")

		// Verify all records still exist in database (found with Unscoped)
		var countUnscoped int64
		require.NoError(t, database.DB().Unscoped().Model(&TestUser{}).Count(&countUnscoped).Error)
		require.Equal(t, int64(3), countUnscoped, "all records should still exist with Unscoped")

		// Verify all records have deleted_at set
		var users []TestUser
		require.NoError(t, database.DB().Unscoped().Find(&users).Error)
		require.Len(t, users, 3)
		for _, u := range users {
			require.NotNil(t, u.DeletedAt, "soft-deleted record should have deleted_at set")
		}
	})

	t.Run("DoesNotAffectCreate", func(t *testing.T) {
		defer cleanupTestData()

		// WithPurge should not affect Create operations
		// Create with WithPurge() - should work normally
		require.NoError(t, database.Database[*TestUser](nil).WithPurge().Create(ul...))
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithPurge()")

		// Create with WithPurge(true) - should work normally
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).Create(ul...))
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithPurge(true)")

		// Create with WithPurge(false) - should work normally
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).Create(ul...))
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithPurge(false)")
	})

	t.Run("DoesNotAffectUpdate", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge should not affect Update operations
		u1.Name = "updated1"
		u2.Name = "updated2"
		require.NoError(t, database.Database[*TestUser](nil).WithPurge().Update(u1, u2))
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).Update(u1, u2))
		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).Update(u1, u2))

		// Verify records are updated successfully
		u11 := new(TestUser)
		u22 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(u11, u1.ID))
		require.NoError(t, database.Database[*TestUser](nil).Get(u22, u2.ID))
		require.Equal(t, "updated1", u11.Name)
		require.Equal(t, "updated2", u22.Name)
	})

	t.Run("DoesNotAffectList", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge should not affect List operations
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithPurge().List(&users))
		require.Len(t, users, 3, "WithPurge should not affect List")

		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).List(&users))
		require.Len(t, users, 3, "WithPurge(true) should not affect List")

		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).List(&users))
		require.Len(t, users, 3, "WithPurge(false) should not affect List")
	})

	t.Run("DoesNotAffectGet", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithPurge should not affect Get operations
		u := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithPurge().Get(u, u1.ID))
		require.NotNil(t, u)
		require.Equal(t, u1.ID, u.ID)

		require.NoError(t, database.Database[*TestUser](nil).WithPurge(true).Get(u, u2.ID))
		require.Equal(t, u2.ID, u.ID)

		require.NoError(t, database.Database[*TestUser](nil).WithPurge(false).Get(u, u3.ID))
		require.Equal(t, u3.ID, u.ID)
	})
}

func TestDatabaseWithCache(t *testing.T) {
	defer cleanupTestData()

	t.Run("DoesNotAffectCreate", func(t *testing.T) {
		defer cleanupTestData()

		// WithCache should not affect Create operations
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Create(ul...))
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithCache()")

		// Create with WithCache(true) - should work normally
		require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))
		require.NoError(t, database.Database[*TestUser](nil).WithCache(true).Create(ul...))
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithCache(true)")

		// Create with WithCache(false) - should work normally
		require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).Create(ul...))
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records after Create with WithCache(false)")
	})

	t.Run("DoesNotAffectDelete", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithCache should not affect Delete operations (but Delete clears cache)
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(3), *count, "should have 3 records initially")

		require.NoError(t, database.Database[*TestUser](nil).WithCache().Delete(u1))
		require.NoError(t, database.Database[*TestUser](nil).Count(count))
		require.Equal(t, int64(2), *count, "should have 2 records after Delete with WithCache()")
	})

	t.Run("DoesNotAffectUpdate", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithCache should not affect Update operations (but Update clears cache)
		u1.Name = "updated1"
		u2.Name = "updated2"
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Update(u1, u2))

		// Verify records are updated successfully
		u11 := new(TestUser)
		u22 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(u11, u1.ID))
		require.NoError(t, database.Database[*TestUser](nil).Get(u22, u2.ID))
		require.Equal(t, "updated1", u11.Name)
		require.Equal(t, "updated2", u22.Name)
	})

	t.Run("List", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test List with cache enabled - results should be consistent
		users1 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).List(&users1))
		require.Len(t, users1, 3, "should have 3 records without cache")

		users2 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().List(&users2))
		require.Len(t, users2, 3, "should have 3 records with cache enabled")
		require.True(t, reflect.DeepEqual(users1, users2), "results should be identical")

		// Test List with cache disabled - should still work
		users3 := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).List(&users3))
		require.Len(t, users3, 3, "should have 3 records with cache disabled")
		require.True(t, reflect.DeepEqual(users1, users3), "results should be identical")
	})

	t.Run("Get", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test Get with cache enabled - results should be consistent
		uu1 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Get(uu1, u1.ID))
		require.NotNil(t, uu1)
		require.Equal(t, u1.ID, uu1.ID)

		uu2 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Get(uu2, u1.ID))
		require.NotNil(t, uu2)
		require.True(t, reflect.DeepEqual(uu1, uu2), "results should be identical")

		// Test Get with cache disabled - should still work
		uu3 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).Get(uu3, u1.ID))
		require.NotNil(t, uu3)
		require.True(t, reflect.DeepEqual(uu1, uu3), "results should be identical")
	})

	t.Run("Count", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test Count with cache enabled
		count1 := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).Count(count1))
		require.Equal(t, int64(3), *count1, "should have 3 records without cache")

		count2 := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Count(count2))
		require.Equal(t, int64(3), *count2, "should have 3 records with cache enabled")
		require.Equal(t, *count1, *count2, "counts should be identical")

		// Test Count with cache disabled - should still work
		count3 := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).Count(count3))
		require.Equal(t, int64(3), *count3, "should have 3 records with cache disabled")
		require.Equal(t, *count1, *count3, "counts should be identical")
	})

	t.Run("First", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test First with cache enabled - results should be consistent
		uu1 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).First(uu1))
		require.NotNil(t, uu1)
		require.NotEmpty(t, uu1.ID)

		uu2 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().First(uu2))
		require.NotNil(t, uu2)
		require.True(t, reflect.DeepEqual(uu1, uu2), "results should be identical")

		// Test First with cache disabled - should still work
		uu3 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).First(uu3))
		require.NotNil(t, uu3)
		require.True(t, reflect.DeepEqual(uu1, uu3), "results should be identical")
	})

	t.Run("Last", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test Last with cache enabled - results should be consistent
		uu1 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Last(uu1))
		require.NotNil(t, uu1)
		require.NotEmpty(t, uu1.ID)

		uu2 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Last(uu2))
		require.NotNil(t, uu2)
		require.True(t, reflect.DeepEqual(uu1, uu2), "results should be identical")

		// Test Last with cache disabled - should still work
		uu3 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).Last(uu3))
		require.NotNil(t, uu3)
		require.True(t, reflect.DeepEqual(uu1, uu3), "results should be identical")
	})

	t.Run("Take", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// Test Take with cache enabled - results should be consistent
		uu1 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).Take(uu1))
		require.NotNil(t, uu1)
		require.NotEmpty(t, uu1.ID)

		uu2 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache().Take(uu2))
		require.NotNil(t, uu2)
		require.True(t, reflect.DeepEqual(uu1, uu2), "results should be identical")

		// Test Take with cache disabled - should still work
		uu3 := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithCache(false).Take(uu3))
		require.NotNil(t, uu3)
		require.True(t, reflect.DeepEqual(uu1, uu3), "results should be identical")
	})
}

func TestDatabaseWithOmit(t *testing.T) {
	defer cleanupTestData()

	t.Run("Create", func(t *testing.T) {
		t.Run("OmitName", func(t *testing.T) {
			defer cleanupTestData()

			require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)

			for _, u := range users {
				require.NotNil(t, u)
				require.Empty(t, u.Name, "name should be omitted")
				require.NotEmpty(t, u.Age, "age should not be empty")
				require.NotEmpty(t, u.Email, "email should not be empty")
				require.NotEmpty(t, u.CreatedAt, "created_at should not be empty")
				require.NotEmpty(t, u.UpdatedAt, "updated_at should not be empty")
			}
		})

		t.Run("OmitAge", func(t *testing.T) {
			defer cleanupTestData()

			require.NoError(t, database.Database[*TestUser](nil).WithOmit("age").Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)

			for _, u := range users {
				require.NotNil(t, u)
				require.Empty(t, u.Age, "age should be omitted")
				require.NotEmpty(t, u.Name, "name should not be empty")
				require.NotEmpty(t, u.Email, "email should not be empty")
				require.NotEmpty(t, u.CreatedAt, "created_at should not be empty")
				require.NotEmpty(t, u.UpdatedAt, "updated_at should not be empty")
			}
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("OmitName", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			// Update with omit name - name should remain unchanged
			originalName := u1.Name
			u1.Age = 25
			require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").Update(u1))

			uu := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(uu, u1.ID))
			require.Equal(t, originalName, uu.Name, "name should remain unchanged when omitted")
			require.Equal(t, int(25), uu.Age, "age should be updated")
		})

		t.Run("OmitAge", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			// Update with omit age - age should remain unchanged
			originalAge := u1.Age
			u1.Name = "updated_name"
			require.NoError(t, database.Database[*TestUser](nil).WithOmit("age").Update(u1))

			uu := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(uu, u1.ID))
			require.Equal(t, originalAge, uu.Age, "age should remain unchanged when omitted")
			require.Equal(t, "updated_name", uu.Name, "name should be updated")
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("SoftDelete", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			// WithOmit should not affect Delete operation (soft delete)
			count := new(int64)
			require.NoError(t, database.Database[*TestUser](nil).Count(count))
			require.Equal(t, int64(3), *count, "should have 3 records initially")

			require.NoError(t, database.Database[*TestUser](nil).WithOmit("name", "age").Delete(u1))
			require.NoError(t, database.Database[*TestUser](nil).Count(count))
			require.Equal(t, int64(2), *count, "should have 2 records after soft delete")

			// Verify record is soft-deleted (not accessible via Get)
			uu := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(uu, u1.ID))
			require.Empty(t, uu.ID, "soft-deleted record should not be accessible via Get")
		})

		t.Run("HardDelete", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			// WithOmit should not affect Delete operation (hard delete)
			count := new(int64)
			require.NoError(t, database.Database[*TestUser](nil).Count(count))
			require.Equal(t, int64(3), *count, "should have 3 records initially")

			require.NoError(t, database.Database[*TestUser](nil).WithOmit("name", "age").WithPurge().Delete(u1))
			require.NoError(t, database.Database[*TestUser](nil).Count(count))
			require.Equal(t, int64(2), *count, "should have 2 records after hard delete")

			// Verify record is permanently deleted (not accessible via Get)
			uu := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(uu, u1.ID))
			require.Empty(t, uu.ID, "hard-deleted record should not be accessible via Get")
		})
	})

	t.Run("List", func(t *testing.T) {
		t.Run("OmitName", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").List(&users))
			require.Len(t, users, 3)

			for _, u := range users {
				require.NotNil(t, u)
				require.Empty(t, u.Name, "name should be omitted")
				require.NotEmpty(t, u.Age, "age should not be empty")
				require.NotEmpty(t, u.Email, "email should not be empty")
			}
		})

		t.Run("OmitAge", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).WithOmit("age").List(&users))
			require.Len(t, users, 3)

			for _, u := range users {
				require.NotNil(t, u)
				require.NotEmpty(t, u.Name, "name should not be empty")
				require.Empty(t, u.Age, "age should be omitted")
				require.NotEmpty(t, u.Email, "email should not be empty")
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").Get(uu, u1.ID))
		require.NotNil(t, uu)
		require.Equal(t, u1.ID, uu.ID)
		require.Empty(t, uu.Name, "name should be omitted")
		require.NotEmpty(t, uu.Age, "age should not be empty")
		require.NotEmpty(t, uu.Email, "email should not be empty")
	})

	t.Run("Count", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithOmit should not affect Count operation
		count := new(int64)
		require.NoError(t, database.Database[*TestUser](nil).WithOmit("name", "age").Count(count))
		require.Equal(t, int64(3), *count, "count should not be affected by WithOmit")
	})

	t.Run("First", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").First(uu))
		require.NotNil(t, uu)
		require.NotEmpty(t, uu.ID)
		require.Empty(t, uu.Name, "name should be omitted")
		require.NotEmpty(t, uu.Age, "age should not be empty")
		require.NotEmpty(t, uu.Email, "email should not be empty")
	})

	t.Run("Last", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithOmit("age").Last(uu))
		require.NotNil(t, uu)
		require.NotEmpty(t, uu.ID)
		require.NotEmpty(t, uu.Name, "name should not be empty")
		require.Empty(t, uu.Age, "age should be omitted")
		require.NotEmpty(t, uu.Email, "email should not be empty")
	})

	t.Run("Take", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](nil).WithOmit("name").Take(uu))
		require.NotNil(t, uu)
		require.NotEmpty(t, uu.ID)
		require.Empty(t, uu.Name, "name should be omitted")
		require.NotEmpty(t, uu.Age, "age should not be empty")
		require.NotEmpty(t, uu.Email, "email should not be empty")
	})
}
