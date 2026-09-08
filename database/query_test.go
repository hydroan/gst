package database_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDatabaseWithQuery(t *testing.T) {
	t.Run("ExactMatch", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test exact match by Name field: query each user by name
		testCases := []struct {
			name     string
			query    *TestUser
			expected *TestUser
		}{
			{"query u1 by name", &TestUser{Name: u1.Name}, u1},
			{"query u2 by name", &TestUser{Name: u2.Name}, u2},
			{"query u3 by name", &TestUser{Name: u3.Name}, u3},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				users = make([]*TestUser, 0)
				require.NoError(t, database.Database[*TestUser](context.Background()).
					WithQuery(tc.query).
					List(&users))
				require.Len(t, users, 1)
				u := users[0]
				require.NotNil(t, u)
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, tc.expected.ID, u.ID)
				require.Equal(t, tc.expected.Name, u.Name)
				require.Equal(t, tc.expected.Age, u.Age)
				require.Equal(t, tc.expected.Email, u.Email)
				require.Equal(t, tc.expected.IsActive, u.IsActive)
			})
		}

		// Test exact match by Age field: query each user by age
		ageTestCases := []struct {
			name     string
			query    *TestUser
			expected *TestUser
		}{
			{"query u1 by age", &TestUser{Age: u1.Age}, u1},
			{"query u2 by age", &TestUser{Age: u2.Age}, u2},
			{"query u3 by age", &TestUser{Age: u3.Age}, u3},
		}

		for _, tc := range ageTestCases {
			t.Run(tc.name, func(t *testing.T) {
				users = make([]*TestUser, 0)
				require.NoError(t, database.Database[*TestUser](context.Background()).
					WithQuery(tc.query).
					List(&users))
				require.Len(t, users, 1)
				u := users[0]
				require.NotNil(t, u)
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, tc.expected.ID, u.ID)
				require.Equal(t, tc.expected.Name, u.Name)
				require.Equal(t, tc.expected.Age, u.Age)
				require.Equal(t, tc.expected.Email, u.Email)
				require.Equal(t, tc.expected.IsActive, u.IsActive)
			})
		}

		// Test exact match by Email field: query each user by email
		emailTestCases := []struct {
			name     string
			query    *TestUser
			expected *TestUser
		}{
			{"query u1 by email", &TestUser{Email: u1.Email}, u1},
			{"query u2 by email", &TestUser{Email: u2.Email}, u2},
			{"query u3 by email", &TestUser{Email: u3.Email}, u3},
		}

		for _, tc := range emailTestCases {
			t.Run(tc.name, func(t *testing.T) {
				users = make([]*TestUser, 0)
				require.NoError(t, database.Database[*TestUser](context.Background()).
					WithQuery(tc.query).
					List(&users))
				require.Len(t, users, 1)
				u := users[0]
				require.NotNil(t, u)
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, tc.expected.ID, u.ID)
				require.Equal(t, tc.expected.Name, u.Name)
				require.Equal(t, tc.expected.Age, u.Age)
				require.Equal(t, tc.expected.Email, u.Email)
				require.Equal(t, tc.expected.IsActive, u.IsActive)
			})
		}

		// Test exact match with multiple fields (AND logic): Name and Age
		// Query: Name="user1" AND Age=18 should return u1
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: u1.Name, Age: u1.Age}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)
		require.Equal(t, u1.Email, users[0].Email)

		// Test exact match with multiple fields that don't match: Name="user1" AND Age=19
		// Should return 0 records (no user matches both)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: u1.Name, Age: u2.Age}).
			List(&users))
		require.Empty(t, users, "multiple fields with AND logic should match all conditions")

		// Test exact match with three fields: Name, Age, and Email
		// Query: Name="user1" AND Age=18 AND Email="user1@example.com" should return u1
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: u1.Name, Age: u1.Age, Email: u1.Email}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)
		require.Equal(t, u1.Email, users[0].Email)

		// Test exact match with non-existent value: should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: "nonexistent"}).
			List(&users))
		require.Empty(t, users, "non-existent value should return 0 records")

		// Test exact match with non-existent age: should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Age: 999}).
			List(&users))
		require.Empty(t, users, "non-existent age should return 0 records")
	})

	t.Run("CommaIsData", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// A comma inside a field value is data, never a list separator: the
		// value binds as one literal, so a record whose value contains a comma
		// stays queryable and two names never ride in on one string. An
		// explicit list of values goes through the in operator filter.
		commaUser := &TestUser{Name: "user1,user2", Email: "comma@example.com", Age: 40, ID: "u-comma"}
		require.NoError(t, database.Database[*TestUser](context.Background()).Create(commaUser))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: "user1,user2"}).
			List(&users))
		require.Len(t, users, 1, "the comma value must match literally, not expand into a list")
		require.Equal(t, commaUser.ID, users[0].ID)

		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{types.FilterIn("id", []string{u1.ID, u2.ID})}}).
			List(&users))
		require.Len(t, users, 2, "an explicit list of values is the in filter's job")
	})

	t.Run("AllowEmpty", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test nil query without AllowEmpty (should return no records, blocked for safety)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(nil).List(&users))
		require.Empty(t, users, "nil query should be blocked by default")

		// Test empty struct without AllowEmpty (should return no records, blocked for safety)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithQuery(&TestUser{}).List(&users))
		require.Empty(t, users, "empty struct should be blocked by default")

		// Test query with all empty string fields without AllowEmpty (should return no records)
		// This tests the second check point where all field values are empty strings
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: "", Email: ""}).
			List(&users))
		require.Empty(t, users, "query with all empty string fields should be blocked by default")

		// Test nil query with AllowEmpty=true (should return all records)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true}).
			List(&users))
		require.Len(t, users, 3)
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
				require.Equal(t, u1.IsActive, u.IsActive)
			case u2.ID:
				foundU2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Age, u.Age)
				require.Equal(t, u2.Email, u.Email)
				require.Equal(t, u2.IsActive, u.IsActive)
			case u3.ID:
				foundU3 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u3.Name, u.Name)
				require.Equal(t, u3.Age, u.Age)
				require.Equal(t, u3.Email, u.Email)
				require.Equal(t, u3.IsActive, u.IsActive)
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test empty struct with AllowEmpty=true (should return all records)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{}, types.QueryOptions{AllowEmpty: true}).
			List(&users))
		require.Len(t, users, 3)
		foundU1, foundU2, foundU3 = false, false, false
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				foundU1 = true
			case u2.ID:
				foundU2 = true
			case u3.ID:
				foundU3 = true
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test query with all empty string fields with AllowEmpty=true (should return all records)
		// This tests the second check point with AllowEmpty=true
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: "", Email: ""}, types.QueryOptions{AllowEmpty: true}).
			List(&users))
		require.Len(t, users, 3, "query with all empty string fields should return all records when AllowEmpty=true")
		foundU1, foundU2, foundU3 = false, false, false
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				foundU1 = true
			case u2.ID:
				foundU2 = true
			case u3.ID:
				foundU3 = true
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test query with some empty and some non-empty fields (should work normally, not blocked)
		// Query: Name="user1" (non-empty), Email="" (empty)
		// Should return u1 (matches Name), not blocked because at least one field is non-empty
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: u1.Name, Email: ""}).
			List(&users))
		require.Len(t, users, 1, "query with some non-empty fields should work normally")
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Email, users[0].Email)

		// Test AllowEmpty=false explicitly (should be same as default)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{AllowEmpty: false}).
			List(&users))
		require.Empty(t, users, "AllowEmpty=false should block empty queries")
	})

	t.Run("PresentFields", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		zeroAgeUser := &TestUser{Name: "user0", Email: "user0@example.com", Age: 0, ID: "u0"}
		require.NoError(t, database.Database[*TestUser](context.Background()).Create(zeroAgeUser))

		// Without presence a zero-value field is treated as unset, so the query
		// stays empty and falls back to the "1 = 0" safety condition.
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Age: 0}).
			List(&users))
		require.Empty(t, users, "zero values without presence should keep the empty-query safety behavior")

		// With presence the zero value becomes a regular condition.
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Age: 0}, types.QueryOptions{PresentFields: map[string]struct{}{"age": {}}}).
			List(&users))
		require.Len(t, users, 1, "an explicitly provided zero value should filter records")
		require.Equal(t, zeroAgeUser.ID, users[0].ID)

		// Presence only unlocks zero values; non-zero fields keep working and
		// combine with AND logic as usual.
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: zeroAgeUser.Name, Age: 0}, types.QueryOptions{PresentFields: map[string]struct{}{"age": {}}}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, zeroAgeUser.ID, users[0].ID)
	})

	t.Run("Filters", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		ids := func(query *TestUser, filters ...types.Filter) []string {
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).
				WithQuery(query, types.QueryOptions{Filters: filters}).
				List(&users))
			out := make([]string, 0, len(users))
			for _, u := range users {
				out = append(out, u.ID)
			}
			slices.Sort(out)
			return out
		}

		// Filters are real conditions on their own: a nil model and an empty
		// model both skip the empty-query safety condition once a filter is
		// present, and the filter alone decides the rows.
		require.Equal(t, []string{u2.ID, u3.ID}, ids(nil, types.FilterGt("age", 18)))
		require.Equal(t, []string{u2.ID, u3.ID}, ids(&TestUser{}, types.FilterGte("age", 19)))

		// A range is two bounds, a set is an explicit list and a disjunction
		// is an OR group, so no condition needs a SQL fragment.
		require.Equal(t, []string{u2.ID}, ids(nil, types.FilterGte("age", 19), types.FilterLte("age", 19)))
		require.Equal(t, []string{u1.ID, u3.ID}, ids(nil, types.FilterIn("age", []int{18, 20})))
		require.Equal(t, []string{u1.ID, u2.ID}, ids(nil, types.FilterOr(types.FilterEq("name", u1.Name), types.FilterEq("age", u2.Age))))
		require.Equal(t, []string{u2.ID}, ids(nil,
			types.FilterOr(types.FilterEq("name", u2.Name), types.FilterEq("email", u2.Email)),
			types.FilterGte("age", 19),
		))

		// Filters and model fields combine with AND: the model narrows what
		// the filters admit, in both directions.
		require.Empty(t, ids(&TestUser{Name: u1.Name}, types.FilterGt("age", 18)))
		require.Equal(t, []string{u1.ID}, ids(&TestUser{Name: u1.Name}, types.FilterGte("age", 18)))
		require.Equal(t, []string{u2.ID}, ids(&TestUser{Name: u2.Name, Email: u2.Email}, types.FilterGt("age", 18)))

		// A filter that matches nothing answers with no rows, not an error.
		require.Empty(t, ids(nil, types.FilterGt("age", 100)))
	})

	t.Run("AutoBase", func(t *testing.T) {
		defer cleanupTestData()
		items := []*TestAutoItem{
			{Code: "query-a1", Name: "first"},
			{Code: "query-a2", Name: "second"},
		}
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).Create(items...))

		// Filter by the embedded auto increment id.
		got := make([]*TestAutoItem, 0)
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).
			WithQuery(&TestAutoItem{ID: items[0].ID}).
			List(&got))
		require.Len(t, got, 1)
		require.Equal(t, items[0].ID, got[0].ID)
		require.Equal(t, items[0].Code, got[0].Code)

		// Filter by a regular column on an AutoBase model.
		got = make([]*TestAutoItem, 0)
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).
			WithQuery(&TestAutoItem{Code: items[1].Code}).
			List(&got))
		require.Len(t, got, 1)
		require.Equal(t, items[1].ID, got[0].ID)

		// DeletedAt on the embedded base must not leak bogus conditions into the query.
		got = make([]*TestAutoItem, 0)
		require.NoError(t, database.Database[*TestAutoItem](context.Background()).
			WithQuery(&TestAutoItem{DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}).
			List(&got))
	})
}
