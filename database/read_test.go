package database_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestDatabaseList(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic List - should return all records
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3, "should have 3 records")

	// Verify all records are returned correctly
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
	require.NotEmpty(t, u11.CreatedAt)
	require.NotEmpty(t, u22.CreatedAt)
	require.NotEmpty(t, u33.CreatedAt)
	require.NotEmpty(t, u11.UpdatedAt)
	require.NotEmpty(t, u22.UpdatedAt)
	require.NotEmpty(t, u33.UpdatedAt)
	require.NotEmpty(t, u11.ID)
	require.NotEmpty(t, u22.ID)
	require.NotEmpty(t, u33.ID)
	require.Equal(t, u1.Name, u11.Name)
	require.Equal(t, u2.Name, u22.Name)
	require.Equal(t, u3.Name, u33.Name)
	require.Equal(t, u1.Age, u11.Age)
	require.Equal(t, u2.Age, u22.Age)
	require.Equal(t, u3.Age, u33.Age)
	require.Equal(t, u1.Email, u11.Email)
	require.Equal(t, u2.Email, u22.Email)
	require.Equal(t, u3.Email, u33.Email)
	require.Equal(t, u1.IsActive, u11.IsActive)
	require.Equal(t, u2.IsActive, u22.IsActive)
	require.Equal(t, u3.IsActive, u33.IsActive)

	// Test List with query conditions
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Name: u1.Name}).List(&users))
	require.Len(t, users, 1, "should have 1 record matching name")
	require.Equal(t, u1.Name, users[0].Name)

	// Test List with Remark field query condition using TestUser2 (no hooks)
	testRemark := "test remark for query"
	u2_1 := &TestUser2{
		Name:  "user2_1",
		Email: "user2_1@example.com",
		Age:   25,
		Base:  model.Base{ID: "u2_1"},
	}
	u2_1.Remark = &testRemark
	require.NoError(t, database.Database[*TestUser2](context.Background()).Create(u2_1))

	u2_2 := &TestUser2{
		Name:  "user2_2",
		Email: "user2_2@example.com",
		Age:   26,
		Base:  model.Base{ID: "u2_2"},
	}
	// u2_2 has no remark (nil)
	require.NoError(t, database.Database[*TestUser2](context.Background()).Create(u2_2))

	// Verify u2_1 was created with remark
	u2_1Check := new(TestUser2)
	require.NoError(t, database.Database[*TestUser2](context.Background()).Get(u2_1Check, u2_1.ID))
	require.NotNil(t, u2_1Check.Remark, "u2_1 should have remark")
	require.Equal(t, testRemark, *u2_1Check.Remark, "u2_1 remark should match")

	// Query by Remark field - should find u2_1
	queryUser2 := &TestUser2{}
	queryUser2.Remark = &testRemark
	users2 := make([]*TestUser2, 0)
	require.NoError(t, database.Database[*TestUser2](context.Background()).WithQuery(queryUser2).List(&users2))
	require.GreaterOrEqual(t, len(users2), 1, "should have at least 1 record matching remark")
	found := false
	for _, u := range users2 {
		if u.ID == u2_1.ID && u.Remark != nil && *u.Remark == testRemark {
			found = true
			break
		}
	}
	require.True(t, found, "should find u2_1 with matching remark")

	// Clean up TestUser2 data
	require.NoError(t, database.Database[*TestUser2](context.Background()).Delete(u2_1, u2_2))

	// Test List after soft delete - should not return soft-deleted records
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 2, "should have 2 records after soft delete")

	// Test List with empty result - should overwrite existing slice
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))
	users = make([]*TestUser, 0, len(ul))
	users = append(users, u1, u2, u3) // Pre-populate with data
	require.Len(t, users, 3, "slice should have 3 items before List")
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "slice should be empty after List with no records")

	// Test List multiple times - should be idempotent
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(ul...))
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3)
	users3 := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users3))
	require.Len(t, users3, 3)

	// Test List with different model types
	products := make([]*TestProduct, 0)
	require.NoError(t, database.Database[*TestProduct](context.Background()).List(&products))

	// Test List with nil dest - should return error
	err := database.Database[*TestUser](context.Background()).List(nil)
	require.Error(t, err, "should return error when dest is nil")
	require.ErrorIs(t, err, database.ErrNilDest, "error should be ErrNilDest")
}

func TestDatabaseListWithJSONString(t *testing.T) {
	defer cleanupTestData()

	data := []*TestUser{
		{Name: "shanghai", Addr: []string{"shanghai1", "shanghai2"}},
		{Name: "beijing", Addr: []string{"beijing1", "beijing2"}},
	}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(data...))

	res := make([]*TestUser, 0)
	// Test query JSON field without fuzzy match.
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(&TestUser{Addr: []string{"shanghai"}}, types.QueryConfig{FuzzyMatch: false}).
		List(&res))
	require.Empty(t, res)

	// Test query JSON field with fuzzy match
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(&TestUser{Addr: []string{"shanghai"}}, types.QueryConfig{FuzzyMatch: true}).
		List(&res))
	require.Len(t, res, 1)
	require.Equal(t, "shanghai", res[0].Name)

	// Test query JSON field with fuzzy match again
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(&TestUser{Addr: []string{"1"}}, types.QueryConfig{FuzzyMatch: true}).
		List(&res))
	require.Len(t, res, 2)
	var found1, found2 bool
	for _, r := range res {
		if r.Name == "shanghai" {
			found1 = true
		}
		if r.Name == "beijing" {
			found2 = true
		}
	}
	require.True(t, found1)
	require.True(t, found2)
}

func TestDatabaseGet(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic Get - should return record by ID
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, u1.ID, u.ID, "should return u1 by ID")
	require.Equal(t, u1.Name, u.Name)
	require.Equal(t, u1.Age, u.Age)
	require.Equal(t, u1.Email, u.Email)

	var stackUser TestUser
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(&stackUser, u1.ID))
	require.Equal(t, u1.ID, stackUser.ID, "should accept an addressable model value")

	// Test Get with different IDs
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u2.ID))
	require.Equal(t, u2.ID, u.ID, "should return u2 by ID")
	require.Equal(t, u2.Name, u.Name)
	require.Equal(t, u2.Age, u.Age)
	require.Equal(t, u2.Email, u.Email)

	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u3.ID))
	require.Equal(t, u3.ID, u.ID, "should return u3 by ID")
	require.Equal(t, u3.Name, u.Name)
	require.Equal(t, u3.Age, u.Age)
	require.Equal(t, u3.Email, u.Email)

	// Test Get with empty ID - should return error
	u = new(TestUser)
	err := database.Database[*TestUser](context.Background()).Get(u, "")
	require.Error(t, err, "should return error when id is empty")
	require.ErrorIs(t, err, database.ErrIDRequired, "error should be ErrIDRequired")

	// Test Get with non-existent ID - should return not found error
	u = new(TestUser)
	err = database.Database[*TestUser](context.Background()).Get(u, "non-existent-id")
	require.ErrorIs(t, err, database.ErrRecordNotFound)
	require.Empty(t, u.ID)
	require.Empty(t, u.CreatedAt)
	require.Empty(t, u.UpdatedAt)
	require.Empty(t, u.Name)
	require.Empty(t, u.Age)
	require.Empty(t, u.Email)

	// Test Get after soft delete - should return not found error
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	u = new(TestUser)
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).Get(u, u1.ID), database.ErrRecordNotFound)
	require.Empty(t, u.ID)
	require.Empty(t, u.CreatedAt)
	require.Empty(t, u.UpdatedAt)
	require.Empty(t, u.Name)
	require.Empty(t, u.Age)
	require.Empty(t, u.Email)

	// Test Get multiple times - should be idempotent
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u2.ID))
	require.Equal(t, u2.ID, u.ID)
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(u, u2.ID))
	require.Equal(t, u2.ID, u.ID)

	// Test Get with different model types
	p := new(TestProduct)
	require.ErrorIs(t, database.Database[*TestProduct](context.Background()).Get(p, "non-existent-id"), database.ErrRecordNotFound)
	require.Empty(t, p.ID)
	require.Empty(t, p.CreatedAt)
	require.Empty(t, p.UpdatedAt)

	// Test Get with nil dest - should returns error
	var uu *TestUser
	err = database.Database[*TestUser](context.Background()).Get(uu, u1.ID)
	require.Error(t, err, "should return error when dest is nil")
	require.ErrorIs(t, err, database.ErrNilDest)
}

func TestDatabaseFirst(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic First - should return first record ordered by primary key
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).First(u))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, u1.Name, u.Name, "should return u1 (first record)")
	require.Equal(t, u1.Age, u.Age)
	require.Equal(t, u1.Email, u.Email)

	// Test First with query conditions
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Name: u2.Name}).First(u))
	require.NotNil(t, u)
	require.Equal(t, u2.Name, u.Name, "should return u2 when querying by name")

	// Test First after soft delete - should not return soft-deleted records
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).First(u))
	require.NotNil(t, u)
	require.Equal(t, u2.Name, u.Name, "should return u2 after u1 is soft-deleted")

	// Test First multiple times - should be idempotent
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).First(u))
	require.Equal(t, u2.Name, u.Name)
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).First(u))
	require.Equal(t, u2.Name, u.Name)

	// Test First with different model types
	p := new(TestProduct)
	err := database.Database[*TestProduct](context.Background()).First(p)
	// First may return error if no records exist, which is acceptable
	if err != nil {
		require.Contains(t, err.Error(), "record not found", "should return 'record not found' error when no records exist")
	}

	// Test First with nil dest - should return error
	var nilFirst *TestUser
	err = database.Database[*TestUser](context.Background()).First(nilFirst)
	require.Error(t, err, "should return error when dest is nil")
	require.ErrorIs(t, err, database.ErrNilDest)
}

func TestDatabaseLast(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic Last - should return last record ordered by primary key
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Last(u))
	require.NotNil(t, u)
	require.NotEmpty(t, u.CreatedAt)
	require.NotEmpty(t, u.UpdatedAt)
	require.NotEmpty(t, u.ID)
	require.Equal(t, u3.Name, u.Name, "should return u3 (last record)")
	require.Equal(t, u3.Age, u.Age)
	require.Equal(t, u3.Email, u.Email)

	// Test Last with query conditions
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Name: u2.Name}).Last(u))
	require.NotNil(t, u)
	require.Equal(t, u2.Name, u.Name, "should return u2 when querying by name")

	// Test Last after soft delete - should not return soft-deleted records
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u3))
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Last(u))
	require.NotNil(t, u)
	require.Equal(t, u2.Name, u.Name, "should return u2 after u3 is soft-deleted")

	// Test Last multiple times - should be idempotent
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Last(u))
	require.Equal(t, u2.Name, u.Name)
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Last(u))
	require.Equal(t, u2.Name, u.Name)

	// Test Last with different model types
	p := new(TestProduct)
	err := database.Database[*TestProduct](context.Background()).Last(p)
	// Last may return error if no records exist, which is acceptable
	if err != nil {
		require.Contains(t, err.Error(), "record not found", "should return 'record not found' error when no records exist")
	}

	// Test Last with nil dest - should return error
	var nilLast *TestUser
	err = database.Database[*TestUser](context.Background()).Last(nilLast)
	require.Error(t, err, "should return error when dest is nil")
	require.ErrorIs(t, err, database.ErrNilDest)
}

func TestDatabaseTake(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test Take - should return a record
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Take(u))
	require.NotEmpty(t, u.ID)

	// Test Take with nil dest - should return error
	var nilTake *TestUser
	err := database.Database[*TestUser](context.Background()).Take(nilTake)
	require.Error(t, err, "should return error when dest is nil")
	require.ErrorIs(t, err, database.ErrNilDest)
}

func TestDatabaseCount(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// Test basic count - should return total number of records
	count := new(int64)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(3), *count, "should have 3 records")

	// Test count with query conditions
	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Name: u1.Name}).Count(count))
	require.Equal(t, int64(1), *count, "should have 1 record matching name")

	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Age: u2.Age}).Count(count))
	require.Equal(t, int64(1), *count, "should have 1 record matching age")

	// Test count after soft delete - soft-deleted records should not be counted
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(u1))
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(2), *count, "should have 2 records after soft delete")

	// Test count with query after soft delete
	require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{Name: u1.Name}).Count(count))
	require.Equal(t, int64(0), *count, "soft-deleted record should not be counted")

	// Test count multiple times - should be idempotent
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(2), *count)
	require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
	require.Equal(t, int64(2), *count)

	// Test count with different model types
	require.NoError(t, database.Database[*TestProduct](context.Background()).Count(count))
	require.GreaterOrEqual(t, *count, int64(0), "product count should be non-negative")
	require.NoError(t, database.Database[*TestCategory](context.Background()).Count(count))
	require.GreaterOrEqual(t, *count, int64(0), "category count should be non-negative")

	// Test count with nil pointer - should return error
	err := database.Database[*TestUser](context.Background()).Count(nil)
	require.Error(t, err, "should return error when count is nil")
	require.Contains(t, err.Error(), "count parameter cannot be nil", "error message should indicate nil pointer issue")
}
