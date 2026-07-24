package database_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

// TestDatabase

func TestDatabaseCreate(t *testing.T) {
	defer cleanupTestData()

	// Test basic Create - single record
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(u1))
	count := new(int)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 1, *count, "should have 1 record after creating single record")

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

	// Test Create - batch create multiple records. u1 already exists and Create
	// no longer overwrites, so drop it before recreating the whole batch.
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	u1.Remark, u2.Remark, u3.Remark = nil, nil, nil // clear remark to test hook
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(ul...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 3, *count, "should have 3 records after batch create")

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
	nilFilterUser := &TestUser{Name: "nil-filter", Base: model.Base{ID: "nil-filter-id"}}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create([]*TestUser{nil, nilFilterUser, nil}...))

	t.Run("duplicated unique key is rejected", func(t *testing.T) {
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
		err := database.Database[*TestUniqueItem](context.Background()).Create(second)
		require.ErrorIs(t, err, database.ErrDuplicatedKey, "create must reject a unique key collision instead of overwriting")

		items := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).List(&items))
		require.Len(t, items, 1)
		require.Equal(t, first.ID, items[0].ID)
		require.Equal(t, "first", items[0].Name, "the existing row must stay untouched")
	})

	t.Run("duplicated key rolls back the whole batch", func(t *testing.T) {
		seed := &TestUniqueItem{UniqueCode: "batch-code", Name: "existing"}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(seed))

		batch := []*TestUniqueItem{
			{UniqueCode: "batch-new", Name: "new"},
			{UniqueCode: "batch-code", Name: "dup"},
		}
		// Batch size 1 splits the two rows into separate INSERT statements, so
		// the rollback must come from the write transaction, not from
		// single-statement atomicity.
		err := database.Database[*TestUniqueItem](context.Background()).WithBatchSize(1).Create(batch...)
		require.ErrorIs(t, err, database.ErrDuplicatedKey)

		rolledBack := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).WithQuery(&TestUniqueItem{UniqueCode: "batch-new"}).List(&rolledBack))
		require.Empty(t, rolledBack, "the row inserted before the collision must roll back")

		seeded := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).WithQuery(&TestUniqueItem{UniqueCode: "batch-code"}).List(&seeded))
		require.Len(t, seeded, 1)
		require.Equal(t, seed.ID, seeded[0].ID)
		require.Equal(t, "existing", seeded[0].Name, "the existing row must stay untouched")
	})

	t.Run("caller supplied timestamps are overridden", func(t *testing.T) {
		past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		item := &TestPlainItem{Code: "ts-code", Name: "ts"}
		item.SetCreatedAt(past)
		item.SetUpdatedAt(past)
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Create(item))
		require.True(t, item.CreatedAt.After(past), "created_at must be forced to now, not the caller value")
		require.True(t, item.UpdatedAt.After(past), "updated_at must be forced to now, not the caller value")
	})

	t.Run("auto increment ids are assigned and backfilled", func(t *testing.T) {
		items := []*TestAutoItem{
			{Code: "create-a1", Name: "first"},
			{Code: "create-a2", Name: "second"},
		}
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Create(items...))
		require.NotZero(t, items[0].ID, "auto increment id should be backfilled after create")
		require.NotZero(t, items[1].ID, "auto increment id should be backfilled after create")
		require.NotEqual(t, items[0].ID, items[1].ID, "each row should get its own id")
		require.NotEmpty(t, items[0].GetID(), "GetID should expose the assigned id")

		single := &TestAutoItem{Code: "create-a3", Name: "third"}
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Create(single))
		require.Greater(t, single.ID, items[1].ID, "later insert should get a larger id")
	})
}

func TestDatabaseDelete(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic Delete - single record (soft delete)
	count := new(int)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 3, *count, "should have 3 records initially")

	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 2, *count, "should have 2 records after soft delete")

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
	require.Equal(t, 0, *count, "should have 0 records after batch soft delete")

	// Verify all records are soft-deleted
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "should have 0 records in List after all soft deleted")

	// Recreate data for next test
	setupTestData(t)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 3, *count, "should have 3 records after recreate")

	// Test Delete - batch delete with slice
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 0, *count, "should have 0 records after batch delete with slice")

	// Test Delete with empty resources - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(nil))
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete([]*TestUser{nil, nil, nil}...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete([]*TestUser{nil, u1, nil}...))

	t.Run("auto increment model rows are deleted by id", func(t *testing.T) {
		items := []*TestAutoItem{
			{Code: "delete-a1"},
			{Code: "delete-a2"},
		}
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Create(items...))
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Delete(items[0]))

		remained := make([]*TestAutoItem, 0)
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).List(&remained))
		require.Len(t, remained, 1)
		require.Equal(t, items[1].ID, remained[0].ID)
	})
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
	count := new(int)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, 3, *count, "should have 3 records after batch update")

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

	// Update is a pure UPDATE: records without an ID are rejected before any
	// database work instead of being inserted.
	now := time.Now().Unix()
	list := []*TestUser{
		{
			Name: strconv.Itoa(int(now)),
		},
	}
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).Update(list...), database.ErrIDRequired)

	// Test Update with empty resources - should not return error
	require.NoError(t, database.Database[*TestUser](context.Background()).Update(nil))
	require.NoError(t, database.Database[*TestUser](context.Background()).Update([]*TestUser{nil, nil, nil}...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Update([]*TestUser{nil, u1, nil}...))

	t.Run("missing record fails with not found", func(t *testing.T) {
		ghost := &TestUser{Name: "ghost", Base: model.Base{ID: "missing-id"}}
		require.ErrorIs(t, database.Database[*TestUser](context.Background()).Update(ghost), database.ErrRecordNotFound)
	})

	t.Run("batch update rolls back when one record is missing", func(t *testing.T) {
		fresh := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(fresh, u1.ID))
		fresh.Name = "batch-rollback"
		ghost := &TestUser{Name: "ghost", Base: model.Base{ID: "missing-id"}}

		err := database.Database[*TestUser](context.Background()).Update(fresh, ghost)
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		reloaded := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(reloaded, u1.ID))
		require.NotEqual(t, "batch-rollback", reloaded.Name, "the row updated before the missing record must roll back")
	})

	t.Run("without hook batch update still rolls back", func(t *testing.T) {
		fresh := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(fresh, u1.ID))
		fresh.Name = "nohook-rollback"
		ghost := &TestUser{Name: "ghost", Base: model.Base{ID: "missing-id"}}

		err := database.Database[*TestUser](context.Background()).WithoutHook().Update(fresh, ghost)
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		reloaded := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(reloaded, u1.ID))
		require.NotEqual(t, "nohook-rollback", reloaded.Name, "WithoutHook writes must keep the transaction boundary")
	})

	t.Run("unchanged values still count as matched", func(t *testing.T) {
		item := &TestPlainItem{Code: "unchanged-code", Name: "unchanged"}
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Create(item))

		// Saving without modifying anything must succeed: matched-rows
		// semantics (clientFoundRows on MySQL) keep this from being misread
		// as a missing record.
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Update(item))
	})

	t.Run("unique key collision fails with duplicated key", func(t *testing.T) {
		first := &TestUniqueItem{UniqueCode: "update-code-a", Name: "first"}
		second := &TestUniqueItem{UniqueCode: "update-code-b", Name: "second"}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(first, second))

		second.UniqueCode = "update-code-a"
		require.ErrorIs(t, database.Database[*TestUniqueItem](context.Background()).Update(second), database.ErrDuplicatedKey)
	})

	t.Run("creation audit columns cannot be forged", func(t *testing.T) {
		item := &TestPlainItem{Code: "audit-code", Name: "before"}
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Create(item))

		persisted := new(TestPlainItem)
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Get(persisted, item.ID))
		originalCreatedAt := persisted.CreatedAt

		persisted.Name = "after"
		persisted.SetCreatedAt(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
		persisted.SetCreatedBy("forged-user")
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Update(persisted))

		reloaded := new(TestPlainItem)
		require.NoError(t, database.Database[*TestPlainItem](context.Background()).Get(reloaded, item.ID))
		require.Equal(t, "after", reloaded.Name)
		require.WithinDuration(t, originalCreatedAt, reloaded.CreatedAt, time.Second, "created_at must not be updatable")
		require.Empty(t, reloaded.CreatedBy, "created_by must not be updatable")
		require.True(t, reloaded.UpdatedAt.After(time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)), "updated_at must be refreshed by the framework")
	})

	t.Run("soft deleted record cannot be updated or resurrected", func(t *testing.T) {
		require.NoError(t, database.DB().AutoMigrate(&TestSoftDeleteItem{}))
		defer func() {
			_ = database.DB().Exec("DELETE FROM test_soft_delete_items").Error
		}()

		item := &TestSoftDeleteItem{Code: "soft-code", Name: "alive"}
		require.NoError(t, database.Database[*TestSoftDeleteItem](context.Background()).Create(item))
		require.NoError(t, database.Database[*TestSoftDeleteItem](context.Background()).Delete(item))

		item.Name = "resurrected"
		require.ErrorIs(t, database.Database[*TestSoftDeleteItem](context.Background()).Update(item), database.ErrRecordNotFound,
			"a soft-deleted row is not a live row for Update")
	})

	t.Run("auto increment model keeps its id on update", func(t *testing.T) {
		item := &TestAutoItem{Code: "update-a1", Name: "before"}
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Create(item))
		created := item.ID
		require.NotZero(t, created)

		item.Name = "after"
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Update(item))
		require.Equal(t, created, item.ID, "update should not change the id")

		items := make([]*TestAutoItem, 0)
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).List(&items))
		require.Len(t, items, 1, "update should not insert a new row")
		require.Equal(t, "after", items[0].Name)
	})
}

func TestDatabaseUpsert(t *testing.T) {
	defer cleanupTestData()

	t.Run("inserts new records", func(t *testing.T) {
		items := []*TestUniqueItem{
			{UniqueCode: "upsert-a", Name: "a"},
			{UniqueCode: "upsert-b", Name: "b"},
		}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Upsert(items...))
		require.NotEmpty(t, items[0].ID)
		require.NotEmpty(t, items[1].ID)

		persisted := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).List(&persisted))
		require.Len(t, persisted, 2)
	})

	t.Run("overwrites on unique key collision and syncs ids", func(t *testing.T) {
		first := &TestUniqueItem{UniqueCode: "upsert-same", Name: "first"}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Upsert(first))
		require.NotEmpty(t, first.ID)

		firstPersisted := new(TestUniqueItem)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Get(firstPersisted, first.ID))

		second := &TestUniqueItem{UniqueCode: "upsert-same", Name: "second"}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Upsert(second))
		require.Equal(t, first.ID, second.ID, "the collided object must expose the persisted row id")
		require.Empty(t, second.CreateAfterID, "Upsert must not run create hooks")
		require.Empty(t, second.UpdateAfterID, "Upsert must not run update hooks")

		items := make([]*TestUniqueItem, 0)
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).WithQuery(&TestUniqueItem{UniqueCode: "upsert-same"}).List(&items))
		require.Len(t, items, 1)
		require.Equal(t, "second", items[0].Name)
		require.WithinDuration(t, firstPersisted.CreatedAt, items[0].CreatedAt, time.Second, "a conflict update keeps the original created_at")
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
