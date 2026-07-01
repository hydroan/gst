package database_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

// TestDatabase

func TestDatabaseCreate(t *testing.T) {
	defer cleanupTestData()

	// Test basic Create - single record
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(u1))
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(1), *count, "should have 1 record after creating single record")

	// Verify single record was created correctly
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.ID, "id should not be empty")
	require.NotEmpty(t, u.CreatedAt, "created_at should not be empty")
	require.NotEmpty(t, u.UpdatedAt, "updated_at should not be empty")
	require.Equal(t, u1.Name, u.Name, "name should match")
	require.Equal(t, u1.Age, u.Age, "age should match")
	require.Equal(t, u1.Email, u.Email, "email should match")

	// Check the create hook result
	require.Equal(t, remarkUserCreateBefore, *u1.Remark, "u1 should have create hook result")

	// Test Create - batch create multiple records
	u1.Remark, u2.Remark, u3.Remark = nil, nil, nil // clear remark to test hook
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(ul...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(3), *count, "should have 3 records after batch create")

	// Check the create hook results for batch create
	require.Equal(t, remarkUserCreateBefore, *u1.Remark, "u1 should have create hook result")
	require.Equal(t, remarkUserCreateBefore, *u2.Remark, "u2 should have create hook result")
	require.Equal(t, remarkUserCreateBefore, *u3.Remark, "u3 should have create hook result")

	// Verify created data in the database
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3, "should have 3 records")
	var u11, u22, u33 *TestUser
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			u11 = u
		case u2.ID:
			u22 = u
		case u3.ID:
			u33 = u
		}
	}
	require.NotNil(t, u11, "u1 should be found")
	require.NotNil(t, u22, "u2 should be found")
	require.NotNil(t, u33, "u3 should be found")
	require.NotEmpty(t, u11.CreatedAt, "u1 created_at should not be empty")
	require.NotEmpty(t, u22.CreatedAt, "u2 created_at should not be empty")
	require.NotEmpty(t, u33.CreatedAt, "u3 created_at should not be empty")
	require.NotEmpty(t, u11.UpdatedAt, "u1 updated_at should not be empty")
	require.NotEmpty(t, u22.UpdatedAt, "u2 updated_at should not be empty")
	require.NotEmpty(t, u33.UpdatedAt, "u3 updated_at should not be empty")
	require.NotEmpty(t, u11.ID, "u1 id should not be empty")
	require.NotEmpty(t, u22.ID, "u2 id should not be empty")
	require.NotEmpty(t, u33.ID, "u3 id should not be empty")
	require.Equal(t, u1.Name, u11.Name, "u1 name should match")
	require.Equal(t, u2.Name, u22.Name, "u2 name should match")
	require.Equal(t, u3.Name, u33.Name, "u3 name should match")
	require.Equal(t, u1.Age, u11.Age, "u1 age should match")
	require.Equal(t, u2.Age, u22.Age, "u2 age should match")
	require.Equal(t, u3.Age, u33.Age, "u3 age should match")
	require.Equal(t, u1.Email, u11.Email, "u1 email should match")
	require.Equal(t, u2.Email, u22.Email, "u2 email should match")
	require.Equal(t, u3.Email, u33.Email, "u3 email should match")
	require.Equal(t, u1.IsActive, u11.IsActive, "u1 is_active should match")
	require.Equal(t, u2.IsActive, u22.IsActive, "u2 is_active should match")
	require.Equal(t, u3.IsActive, u33.IsActive, "u3 is_active should match")

	// all created resources will generates ID automatically
	now := time.Now().Unix()
	list := []*TestUser{
		{
			Name: strconv.Itoa(int(now)),
		},
		{
			Name: strconv.Itoa(int(now + 1)),
		},
		{
			Name: strconv.Itoa(int(now + 2)),
		},
	}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(list...))
	for _, u := range list {
		require.NotEmpty(t, u.ID, "id should not be empty")
	}

	// Test Create with empty resources - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(nil))
	require.NoError(t, database.Database[*TestUser](context.Background()).Create([]*TestUser{nil, nil, nil}...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Create([]*TestUser{nil, u1, nil}...))

	t.Run("syncs upserted unique index record", func(t *testing.T) {
		first := &TestUniqueItem{
			UniqueCode: "same-code",
			Name:       "first",
		}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(first))
		require.NotEmpty(t, first.ID)
		require.Equal(t, first.ID, first.CreateAfterID)

		second := &TestUniqueItem{
			UniqueCode: "same-code",
			Name:       "second",
		}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(second))

		require.Equal(t, first.ID, second.ID, "upserted create should expose the persisted row id")
		require.Equal(t, first.ID, second.CreateAfterID, "CreateAfter should observe the persisted row id")

		items := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).List(&items))
		require.Len(t, items, 1)
		require.Equal(t, first.ID, items[0].ID)
		require.Equal(t, "same-code", items[0].UniqueCode)
		require.Equal(t, "second", items[0].Name)
	})
}

func TestDatabaseDelete(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic Delete - single record (soft delete)
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(3), *count, "should have 3 records initially")

	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(2), *count, "should have 2 records after soft delete")

	// Verify soft-deleted record is not visible in List
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 2, "should have 2 records in List after soft delete")
	var foundU1 bool
	for _, u := range users {
		if u.ID == u1.ID {
			foundU1 = true
		}
	}
	require.False(t, foundU1, "u1 should not be found in List after soft delete")

	// Verify soft-deleted record is not accessible via Get
	u := new(TestUser)
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID), database.ErrRecordNotFound)

	// Test Delete - batch delete multiple records
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u2, u3))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(0), *count, "should have 0 records after batch soft delete")

	// Verify all records are soft-deleted
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "should have 0 records in List after all soft deleted")

	// Recreate data for next test
	setupTestData(t)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(3), *count, "should have 3 records after recreate")

	// Test Delete - batch delete with slice
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(0), *count, "should have 0 records after batch delete with slice")

	// Test Delete with empty resources - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(nil))
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete([]*TestUser{nil, nil, nil}...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete([]*TestUser{nil, u1, nil}...))
}

func TestDatabaseUpdate(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic Update - single record
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, u1.Name, u.Name)
	require.Equal(t, u1.Age, u.Age)
	require.Equal(t, u1.Email, u.Email)

	// Update single record
	u.Name = "user1_updated"
	u.Age = 25
	u.Email = "user1_updated@example.com"
	require.NoError(t, database.Database[*TestUser](context.Background()).Update(u))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, "user1_updated", u.Name, "name should be updated")
	require.Equal(t, 25, u.Age, "age should be updated")
	require.Equal(t, "user1_updated@example.com", u.Email, "email should be updated")

	// Test Update - batch update multiple records
	u1.Name = "user1_batch"
	u2.Name = "user2_batch"
	u3.Name = "user3_batch"
	u1.Remark, u2.Remark, u3.Remark = nil, nil, nil // clear remark to test hook
	require.NoError(t, database.Database[*TestUser](context.Background()).Update(ul...))
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(3), *count, "should have 3 records after batch update")

	// Check the update hook result
	require.Equal(t, remarkUserUpdateBefore, *u1.Remark, "u1 should have update hook result")
	require.Equal(t, remarkUserUpdateBefore, *u2.Remark, "u2 should have update hook result")
	require.Equal(t, remarkUserUpdateBefore, *u3.Remark, "u3 should have update hook result")

	// Verify updated data in the database
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3, "should have 3 records")
	var u11, u22, u33 *TestUser
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			u11 = u
		case u2.ID:
			u22 = u
		case u3.ID:
			u33 = u
		}
	}
	require.NotNil(t, u11, "u1 should be found")
	require.NotNil(t, u22, "u2 should be found")
	require.NotNil(t, u33, "u3 should be found")
	require.NotEmpty(t, u11.CreatedAt, "created_at should not be empty")
	require.NotEmpty(t, u22.CreatedAt, "created_at should not be empty")
	require.NotEmpty(t, u33.CreatedAt, "created_at should not be empty")
	require.NotEmpty(t, u11.UpdatedAt, "updated_at should not be empty")
	require.NotEmpty(t, u22.UpdatedAt, "updated_at should not be empty")
	require.NotEmpty(t, u33.UpdatedAt, "updated_at should not be empty")
	require.NotEmpty(t, u11.ID, "id should not be empty")
	require.NotEmpty(t, u22.ID, "id should not be empty")
	require.NotEmpty(t, u33.ID, "id should not be empty")
	require.Equal(t, u1.Name, u11.Name, "u1 name should match")
	require.Equal(t, u2.Name, u22.Name, "u2 name should match")
	require.Equal(t, u3.Name, u33.Name, "u3 name should match")
	require.Equal(t, u1.Age, u11.Age, "u1 age should match")
	require.Equal(t, u2.Age, u22.Age, "u2 age should match")
	require.Equal(t, u3.Age, u33.Age, "u3 age should match")
	require.Equal(t, u1.Email, u11.Email, "u1 email should match")
	require.Equal(t, u2.Email, u22.Email, "u2 email should match")
	require.Equal(t, u3.Email, u33.Email, "u3 email should match")
	require.Equal(t, u1.IsActive, u11.IsActive, "u1 is_active should match")
	require.Equal(t, u2.IsActive, u22.IsActive, "u2 is_active should match")
	require.Equal(t, u3.IsActive, u33.IsActive, "u3 is_active should match")

	// all created resources will generates ID automatically
	now := time.Now().Unix()
	list := []*TestUser{
		{
			Name: strconv.Itoa(int(now)),
		},
		{
			Name: strconv.Itoa(int(now + 1)),
		},
		{
			Name: strconv.Itoa(int(now + 2)),
		},
	}
	require.NoError(t, database.Database[*TestUser](context.Background()).Update(list...))
	for _, u := range list {
		require.NotEmpty(t, u.ID, "id should not be empty")
	}

	// Test Update with empty resources - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).Update(nil))
	require.NoError(t, database.Database[*TestUser](context.Background()).Update([]*TestUser{nil, nil, nil}...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Update([]*TestUser{nil, u1, nil}...))

	t.Run("syncs upserted unique index record", func(t *testing.T) {
		first := &TestUniqueItem{
			UniqueCode: "update-same-code",
			Name:       "first",
		}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(first))
		require.NotEmpty(t, first.ID)

		second := &TestUniqueItem{
			UniqueCode: "update-same-code",
			Name:       "second",
		}
		second.ID = "update-stale-id"
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Update(second))

		require.Equal(t, first.ID, second.ID, "upserted update should expose the persisted row id")
		require.Equal(t, first.ID, second.UpdateAfterID, "UpdateAfter should observe the persisted row id")

		items := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).List(&items))
		require.Len(t, items, 1)
		require.Equal(t, first.ID, items[0].ID)
		require.Equal(t, "second", items[0].Name)
	})
}

func TestDatabaseUpdateByID(t *testing.T) {
	defer cleanupTestData()

	require.NoError(t, database.Database[*TestUser](context.Background()).Create(u1))
	// Test basic UpdateByID - update name field
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, u1.Name, u.Name)
	require.Equal(t, u1.Age, u.Age)
	require.Equal(t, u1.Email, u.Email)
	originalUpdatedAt := u.UpdatedAt

	newName := "user1_modified"
	require.NoError(t, database.Database[*TestUser](context.Background()).UpdateByID(u.ID, "name", newName))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, newName, u.Name, "name should be updated")
	require.Equal(t, u1.Age, u.Age, "age should not be changed")
	require.Equal(t, u1.Email, u.Email, "email should not be changed")
	require.NotEqual(t, originalUpdatedAt, u.UpdatedAt, "updated_at should be updated")

	// Test UpdateByID - update age field
	newAge := 25
	previousUpdatedAt := u.UpdatedAt
	require.NoError(t, database.Database[*TestUser](context.Background()).UpdateByID(u.ID, "age", newAge))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.Equal(t, newName, u.Name, "name should not be changed")
	require.Equal(t, newAge, u.Age, "age should be updated")
	require.Equal(t, u1.Email, u.Email, "email should not be changed")
	require.NotEqual(t, previousUpdatedAt, u.UpdatedAt, "updated_at should be updated again")

	// Test UpdateByID - update email field
	newEmail := "user1_new@example.com"
	previousUpdatedAt = u.UpdatedAt
	require.NoError(t, database.Database[*TestUser](context.Background()).UpdateByID(u.ID, "email", newEmail))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.Equal(t, newName, u.Name, "name should not be changed")
	require.Equal(t, newAge, u.Age, "age should not be changed")
	require.Equal(t, newEmail, u.Email, "email should be updated")
	require.NotEqual(t, previousUpdatedAt, u.UpdatedAt, "updated_at should be updated again")

	// Test UpdateByID with non-existent ID - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).UpdateByID("non-existent-id", "name", "test"))

	// Test UpdateByID with empty parameters - should return errors
	err := database.Database[*TestUser](context.Background()).UpdateByID("", "name", "value")
	require.Error(t, err, "should return error when id is empty")
	require.ErrorIs(t, err, database.ErrIDRequired, "error should be ErrIDRequired")

	err = database.Database[*TestUser](context.Background()).UpdateByID("id", "", "value")
	require.Error(t, err, "should return error when name is empty")
	require.ErrorIs(t, err, database.ErrEmptyFieldName, "error should be ErrEmptyFieldName")

	err = database.Database[*TestUser](context.Background()).UpdateByID("id", "name", nil)
	require.Error(t, err, "should return error when value is nil")
	require.ErrorIs(t, err, database.ErrNilValue, "error should be ErrNilValue")
}
