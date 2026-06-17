package database_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
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
				require.NoError(t, database.Database[*TestUser](nil).
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
				require.NoError(t, database.Database[*TestUser](nil).
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
				require.NoError(t, database.Database[*TestUser](nil).
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
		require.NoError(t, database.Database[*TestUser](nil).
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
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Age: u2.Age}).
			List(&users))
		require.Empty(t, users, "multiple fields with AND logic should match all conditions")

		// Test exact match with three fields: Name, Age, and Email
		// Query: Name="user1" AND Age=18 AND Email="user1@example.com" should return u1
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Age: u1.Age, Email: u1.Email}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)
		require.Equal(t, u1.Email, users[0].Email)

		// Test exact match with non-existent value: should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "nonexistent"}).
			List(&users))
		require.Empty(t, users, "non-existent value should return 0 records")

		// Test exact match with non-existent age: should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Age: 999}).
			List(&users))
		require.Empty(t, users, "non-existent age should return 0 records")
	})

	t.Run("MultipleValues", func(t *testing.T) {
		t.Run("multiple_id", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)

			// Test multiple IDs with comma-separated values: ID="u1,u2"
			// Should return 2 records (u1 and u2) using IN clause
			query := new(TestUser)
			ids := []string{u1.ID, u2.ID}
			query.ID = strings.Join(ids, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 2)

			var u11, u22 *TestUser
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					u11 = u
				case u2.ID:
					u22 = u
				}
			}
			require.NotNil(t, u11, "should find u1")
			require.NotNil(t, u22, "should find u2")
			require.NotEmpty(t, u11.ID)
			require.NotEmpty(t, u22.ID)
			require.NotEmpty(t, u11.CreatedAt)
			require.NotEmpty(t, u22.CreatedAt)
			require.NotEmpty(t, u11.UpdatedAt)
			require.NotEmpty(t, u22.UpdatedAt)
			require.Equal(t, u1.Name, u11.Name)
			require.Equal(t, u2.Name, u22.Name)
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u1.Email, u11.Email)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u1.IsActive, u11.IsActive)
			require.Equal(t, u2.IsActive, u22.IsActive)

			// Test multiple IDs with three values: ID="u1,u2,u3"
			// Should return all 3 records
			users = make([]*TestUser, 0)
			query = new(TestUser)
			ids = []string{u1.ID, u2.ID, u3.ID}
			query.ID = strings.Join(ids, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 3)
			var foundU1, foundU2, foundU3 bool
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					foundU1 = true
					require.Equal(t, u1.Name, u.Name)
					require.Equal(t, u1.Age, u.Age)
					require.Equal(t, u1.Email, u.Email)
				case u2.ID:
					foundU2 = true
					require.Equal(t, u2.Name, u.Name)
					require.Equal(t, u2.Age, u.Age)
					require.Equal(t, u2.Email, u.Email)
				case u3.ID:
					foundU3 = true
					require.Equal(t, u3.Name, u.Name)
					require.Equal(t, u3.Age, u.Age)
					require.Equal(t, u3.Email, u.Email)
				}
			}
			require.True(t, foundU1, "should find u1")
			require.True(t, foundU2, "should find u2")
			require.True(t, foundU3, "should find u3")

			// Test multiple IDs with non-existent ID: ID="u1,nonexistent"
			// Should return only u1 (non-existent ID is ignored)
			users = make([]*TestUser, 0)
			query = new(TestUser)
			ids = []string{u1.ID, "nonexistent-id"}
			query.ID = strings.Join(ids, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 1)
			require.Equal(t, u1.ID, users[0].ID)
			require.Equal(t, u1.Name, users[0].Name)

			// Test multiple IDs with single value: ID="u1"
			// Should return 1 record (single value should work)
			users = make([]*TestUser, 0)
			query = new(TestUser)
			query.ID = u1.ID
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 1)
			require.Equal(t, u1.ID, users[0].ID)
			require.Equal(t, u1.Name, users[0].Name)
		})

		t.Run("multiple_name", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)

			// Test multiple names with comma-separated values: Name="user2,user3"
			// Should return 2 records (u2 and u3) using IN clause
			query := new(TestUser)
			names := []string{u2.Name, u3.Name}
			query.Name = strings.Join(names, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 2)

			var u22, u33 *TestUser
			for _, u := range users {
				switch u.ID {
				case u2.ID:
					u22 = u
				case u3.ID:
					u33 = u
				}
			}
			require.NotNil(t, u22, "should find u2")
			require.NotNil(t, u33, "should find u3")
			require.NotEmpty(t, u22.ID)
			require.NotEmpty(t, u33.ID)
			require.NotEmpty(t, u22.CreatedAt)
			require.NotEmpty(t, u33.CreatedAt)
			require.NotEmpty(t, u22.UpdatedAt)
			require.NotEmpty(t, u33.UpdatedAt)
			require.Equal(t, u2.Name, u22.Name)
			require.Equal(t, u3.Name, u33.Name)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u3.Age, u33.Age)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u3.Email, u33.Email)
			require.Equal(t, u2.IsActive, u22.IsActive)
			require.Equal(t, u3.IsActive, u33.IsActive)

			// Test multiple names with three values: Name="user1,user2,user3"
			// Should return all 3 records
			users = make([]*TestUser, 0)
			query = new(TestUser)
			names = []string{u1.Name, u2.Name, u3.Name}
			query.Name = strings.Join(names, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 3)
			var foundU1, foundU2, foundU3 bool
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					foundU1 = true
					require.Equal(t, u1.Name, u.Name)
					require.Equal(t, u1.Age, u.Age)
				case u2.ID:
					foundU2 = true
					require.Equal(t, u2.Name, u.Name)
					require.Equal(t, u2.Age, u.Age)
				case u3.ID:
					foundU3 = true
					require.Equal(t, u3.Name, u.Name)
					require.Equal(t, u3.Age, u.Age)
				}
			}
			require.True(t, foundU1, "should find u1")
			require.True(t, foundU2, "should find u2")
			require.True(t, foundU3, "should find u3")

			// Test multiple names with non-existent name: Name="user1,nonexistent"
			// Should return only u1 (non-existent name is ignored)
			users = make([]*TestUser, 0)
			query = new(TestUser)
			names = []string{u1.Name, "nonexistent"}
			query.Name = strings.Join(names, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 1)
			require.Equal(t, u1.ID, users[0].ID)
			require.Equal(t, u1.Name, users[0].Name)

			// Test multiple names with single value: Name="user1"
			// Should return 1 record (single value should work)
			users = make([]*TestUser, 0)
			query = new(TestUser)
			query.Name = u1.Name
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 1)
			require.Equal(t, u1.ID, users[0].ID)
			require.Equal(t, u1.Name, users[0].Name)
		})

		t.Run("multiple_email", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)

			// Test multiple emails with comma-separated values: Email="user1@example.com,user2@example.com"
			// Should return 2 records (u1 and u2) using IN clause
			query := new(TestUser)
			emails := []string{u1.Email, u2.Email}
			query.Email = strings.Join(emails, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 2)

			var u11, u22 *TestUser
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					u11 = u
				case u2.ID:
					u22 = u
				}
			}
			require.NotNil(t, u11, "should find u1")
			require.NotNil(t, u22, "should find u2")
			require.NotEmpty(t, u11.ID)
			require.NotEmpty(t, u22.ID)
			require.NotEmpty(t, u11.CreatedAt)
			require.NotEmpty(t, u22.CreatedAt)
			require.NotEmpty(t, u11.UpdatedAt)
			require.NotEmpty(t, u22.UpdatedAt)
			require.Equal(t, u1.Name, u11.Name)
			require.Equal(t, u2.Name, u22.Name)
			require.Equal(t, u1.Age, u11.Age)
			require.Equal(t, u2.Age, u22.Age)
			require.Equal(t, u1.Email, u11.Email)
			require.Equal(t, u2.Email, u22.Email)
			require.Equal(t, u1.IsActive, u11.IsActive)
			require.Equal(t, u2.IsActive, u22.IsActive)

			// Test multiple emails with three values
			users = make([]*TestUser, 0)
			query = new(TestUser)
			emails = []string{u1.Email, u2.Email, u3.Email}
			query.Email = strings.Join(emails, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 3)
			var foundU1, foundU2, foundU3 bool
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
		})

		t.Run("multiple_fields", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)

			// Test multiple fields with comma-separated values: Name="user1,user2" AND Email="user1@example.com,user2@example.com"
			// Should return 2 records (u1 and u2) - both fields use IN clause with AND logic
			query := new(TestUser)
			names := []string{u1.Name, u2.Name}
			emails := []string{u1.Email, u2.Email}
			query.Name = strings.Join(names, ",")
			query.Email = strings.Join(emails, ",")
			require.NoError(t, database.Database[*TestUser](nil).WithQuery(query).List(&users))
			require.Len(t, users, 2)
			var foundU1, foundU2 bool
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					foundU1 = true
					require.Equal(t, u1.Name, u.Name)
					require.Equal(t, u1.Email, u.Email)
				case u2.ID:
					foundU2 = true
					require.Equal(t, u2.Name, u.Name)
					require.Equal(t, u2.Email, u.Email)
				}
			}
			require.True(t, foundU1, "should find u1")
			require.True(t, foundU2, "should find u2")
		})
	})

	t.Run("FuzzyMatch", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test FuzzyMatch=false (default, exact match): should return 0 records for partial match
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user"}, types.QueryConfig{
				FuzzyMatch: false,
			}).
			List(&users))
		require.Empty(t, users, "FuzzyMatch=false should not match partial strings")

		// Test FuzzyMatch=true with single value (LIKE): query "name" with partial match
		// Should return all 3 records (user1, user2, user3 all contain "user")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3)
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
		require.NotNil(t, u11)
		require.NotNil(t, u22)
		require.NotNil(t, u33)
		require.NotEmpty(t, u11.ID)
		require.NotEmpty(t, u22.ID)
		require.NotEmpty(t, u33.ID)
		require.NotEmpty(t, u11.CreatedAt)
		require.NotEmpty(t, u22.CreatedAt)
		require.NotEmpty(t, u33.CreatedAt)
		require.NotEmpty(t, u11.UpdatedAt)
		require.NotEmpty(t, u22.UpdatedAt)
		require.NotEmpty(t, u33.UpdatedAt)
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

		// Test FuzzyMatch=true with single value (LIKE): query "email" with partial match
		// Should return all 3 records (all emails contain "example.com")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Email: "example.com"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3)
		u11, u22, u33 = nil, nil, nil
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
		require.NotNil(t, u11)
		require.NotNil(t, u22)
		require.NotNil(t, u33)
		require.NotEmpty(t, u11.ID)
		require.NotEmpty(t, u22.ID)
		require.NotEmpty(t, u33.ID)
		require.NotEmpty(t, u11.CreatedAt)
		require.NotEmpty(t, u22.CreatedAt)
		require.NotEmpty(t, u33.CreatedAt)
		require.NotEmpty(t, u11.UpdatedAt)
		require.NotEmpty(t, u22.UpdatedAt)
		require.NotEmpty(t, u33.UpdatedAt)
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

		// Test FuzzyMatch=true with single value (LIKE): exact match should still work
		// Query: Name="user1" should return 1 record (u1)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user1"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 1)
		u := users[0]
		require.NotNil(t, u)
		require.NotEmpty(t, u.CreatedAt)
		require.NotEmpty(t, u.UpdatedAt)
		require.Equal(t, u1.ID, u.ID)
		require.Equal(t, u1.Name, u.Name)
		require.Equal(t, u1.Age, u.Age)
		require.Equal(t, u1.Email, u.Email)
		require.Equal(t, u1.IsActive, u.IsActive)

		// Test FuzzyMatch=true with multiple values (REGEXP): comma-separated values
		// Query: Name="user1,user2" should return 2 records (u1 and u2)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: strings.Join([]string{u1.Name, u2.Name}, ",")}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 2)
		u11, u22 = nil, nil
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				u11 = u
			case u2.ID:
				u22 = u
			}
		}
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u22, "should find u2")
		require.Equal(t, u1.Name, u11.Name)
		require.Equal(t, u2.Name, u22.Name)
		require.Equal(t, u1.Age, u11.Age)
		require.Equal(t, u2.Age, u22.Age)
		require.Equal(t, u1.Email, u11.Email)
		require.Equal(t, u2.Email, u22.Email)

		// Test FuzzyMatch=true with multiple values (REGEXP): partial matches in comma-separated values
		// Query: Name="user,ser" should return all 3 records (all contain "user")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user,ser"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3)
		u11, u22, u33 = nil, nil, nil
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
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u22, "should find u2")
		require.NotNil(t, u33, "should find u3")

		// Test FuzzyMatch=true with no matching value: should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "nonexistent"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Empty(t, users)

		// Test FuzzyMatch=true with empty string: should return 0 records (empty query blocked)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: ""}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Empty(t, users, "empty query should be blocked by default")

		// Test FuzzyMatch=true with multiple fields: Name and Email
		// Query: Name="user" AND Email="example" should return all 3 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user", Email: "example"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3)
		u11, u22, u33 = nil, nil, nil
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
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u22, "should find u2")
		require.NotNil(t, u33, "should find u3")

		// Test FuzzyMatch=true with comma-separated values containing empty strings
		// Query: Name="user1,,user2" should return 2 records (u1 and u2), empty strings should be ignored
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user1,,user2"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 2, "empty strings in comma-separated values should be ignored")
		u11, u22 = nil, nil
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				u11 = u
			case u2.ID:
				u22 = u
			}
		}
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u22, "should find u2")
		require.Equal(t, u1.Name, u11.Name)
		require.Equal(t, u2.Name, u22.Name)
		require.Equal(t, u1.Age, u11.Age)
		require.Equal(t, u2.Age, u22.Age)
		require.Equal(t, u1.Email, u11.Email)
		require.Equal(t, u2.Email, u22.Email)

		// Test FuzzyMatch=true with partial match at start: Name="1" (matches "user1")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "1"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 1, "should match partial string at end")
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)

		// Test FuzzyMatch=true with partial match in middle: Name="ser" (matches all users)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "ser"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3, "should match partial string in middle")
		foundU1, foundU2, foundU3 := false, false, false
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

		// Test FuzzyMatch=true with partial match at end: Name="user" (matches all users)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3, "should match partial string at start")

		// Test FuzzyMatch=true with email partial match: Email="@example" (matches all emails)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Email: "@example"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3, "should match email partial string")
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

		// Test FuzzyMatch=true with REGEXP special characters (should be escaped)
		// Query: Name="user1,user2" with special regex chars should work correctly
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: strings.Join([]string{u1.Name, u2.Name}, ",")}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 2, "REGEXP special characters should be escaped")
		u11, u22 = nil, nil
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				u11 = u
			case u2.ID:
				u22 = u
			}
		}
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u22, "should find u2")

		// Test FuzzyMatch=true with multiple comma-separated values: Name="user1,user3"
		// Should return 2 records (u1 and u3)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: strings.Join([]string{u1.Name, u3.Name}, ",")}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 2)
		u11, u33 = nil, nil
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				u11 = u
			case u3.ID:
				u33 = u
			}
		}
		require.NotNil(t, u11, "should find u1")
		require.NotNil(t, u33, "should find u3")
		require.Equal(t, u1.Name, u11.Name)
		require.Equal(t, u3.Name, u33.Name)
		require.Equal(t, u1.Age, u11.Age)
		require.Equal(t, u3.Age, u33.Age)

		// Test FuzzyMatch=true with AllowEmpty: empty query should return all records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{}, types.QueryConfig{
				FuzzyMatch: true,
				AllowEmpty: true,
			}).
			List(&users))
		require.Len(t, users, 3, "FuzzyMatch with AllowEmpty should return all records")

		// Test FuzzyMatch=true with UseOr: Name="user1" OR Email="user2@example.com"
		// Should return u1 (matches Name) and u2 (matches Email)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Email: u2.Email}, types.QueryConfig{
				FuzzyMatch: true,
				UseOr:      true,
			}).
			List(&users))
		require.Len(t, users, 2, "FuzzyMatch with UseOr should work correctly")
		foundU1, foundU2 = false, false
		for _, u := range users {
			if u.ID == u1.ID {
				foundU1 = true
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Email, u.Email)
			}
			if u.ID == u2.ID {
				foundU2 = true
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Email, u.Email)
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")

		// Test FuzzyMatch=true with single field and empty string value (should be blocked)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "", Email: "example"}, types.QueryConfig{
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3, "query with some non-empty fields should work even with empty strings")

		// Test FuzzyMatch=false explicitly (should be same as default)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user"}, types.QueryConfig{
				FuzzyMatch: false,
			}).
			List(&users))
		require.Empty(t, users, "FuzzyMatch=false should not match partial strings")
	})

	t.Run("AllowEmpty", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test nil query without AllowEmpty (should return no records, blocked for safety)
		require.NoError(t, database.Database[*TestUser](nil).WithQuery(nil).List(&users))
		require.Empty(t, users, "nil query should be blocked by default")

		// Test empty struct without AllowEmpty (should return no records, blocked for safety)
		require.NoError(t, database.Database[*TestUser](nil).WithQuery(&TestUser{}).List(&users))
		require.Empty(t, users, "empty struct should be blocked by default")

		// Test query with all empty string fields without AllowEmpty (should return no records)
		// This tests the second check point where all field values are empty strings
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "", Email: ""}).
			List(&users))
		require.Empty(t, users, "query with all empty string fields should be blocked by default")

		// Test nil query with AllowEmpty=true (should return all records)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{AllowEmpty: true}).
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
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{}, types.QueryConfig{AllowEmpty: true}).
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
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "", Email: ""}, types.QueryConfig{AllowEmpty: true}).
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
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Email: ""}).
			List(&users))
		require.Len(t, users, 1, "query with some non-empty fields should work normally")
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Email, users[0].Email)

		// Test AllowEmpty with FuzzyMatch: should allow empty query when both are enabled
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{}, types.QueryConfig{
				AllowEmpty: true,
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3, "AllowEmpty should work with FuzzyMatch")

		// Test AllowEmpty with UseOr: should allow empty query when both are enabled
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{}, types.QueryConfig{
				AllowEmpty: true,
				UseOr:      true,
			}).
			List(&users))
		require.Len(t, users, 3, "AllowEmpty should work with UseOr")

		// Test AllowEmpty=false explicitly (should be same as default)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{AllowEmpty: false}).
			List(&users))
		require.Empty(t, users, "AllowEmpty=false should block empty queries")
	})

	t.Run("UseOr", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test UseOr=false (default, AND logic): query with multiple fields should return records matching ALL conditions
		// u1: Name="user1", Age=18
		// u2: Name="user2", Age=19
		// u3: Name="user3", Age=20
		// Query: Name="user1" AND Age=19 should return 0 records (no user matches both)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Age: u2.Age}, types.QueryConfig{UseOr: false}).
			List(&users))
		require.Empty(t, users)

		// Test UseOr=false (default, AND logic): query with multiple fields matching same record
		// Query: Name="user1" AND Age=18 should return 1 record (u1 matches both)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Age: u1.Age}, types.QueryConfig{UseOr: false}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)

		// Test UseOr=true (OR logic): query with multiple fields should return records matching ANY condition
		// Query: Name="user1" OR Age=19 should return 2 records (u1 matches Name, u2 matches Age)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Age: u2.Age}, types.QueryConfig{UseOr: true}).
			List(&users))
		require.Len(t, users, 2)
		var foundU1, foundU2 bool
		for _, u := range users {
			if u.ID == u1.ID {
				foundU1 = true
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Age, u.Age)
			}
			if u.ID == u2.ID {
				foundU2 = true
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Age, u.Age)
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")

		// Test UseOr=true with three fields: Name="user1" OR Email="user2@example.com" OR Age=20
		// Should return all 3 records (u1 matches Name, u2 matches Email, u3 matches Age)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name, Email: u2.Email, Age: u3.Age}, types.QueryConfig{UseOr: true}).
			List(&users))
		require.Len(t, users, 3)
		var foundU1_2, foundU2_2, foundU3 bool
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				foundU1_2 = true
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Email, u.Email)
				require.Equal(t, u1.Age, u.Age)
			case u2.ID:
				foundU2_2 = true
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Email, u.Email)
				require.Equal(t, u2.Age, u.Age)
			case u3.ID:
				foundU3 = true
				require.Equal(t, u3.Name, u.Name)
				require.Equal(t, u3.Email, u.Email)
				require.Equal(t, u3.Age, u.Age)
			}
		}
		require.True(t, foundU1_2, "should find u1")
		require.True(t, foundU2_2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test UseOr=true with single field (should work same as UseOr=false for single field)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name}, types.QueryConfig{UseOr: true}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)

		// Test UseOr=true with FuzzyMatch: Name LIKE "%user%" OR Email LIKE "%example%"
		// Should return all 3 records (all match both patterns)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: "user", Email: "example"}, types.QueryConfig{
				UseOr:      true,
				FuzzyMatch: true,
			}).
			List(&users))
		require.Len(t, users, 3)
		foundU1_2, foundU2_2, foundU3 = false, false, false
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				foundU1_2 = true
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Email, u.Email)
				require.Equal(t, u1.Age, u.Age)
			case u2.ID:
				foundU2_2 = true
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Email, u.Email)
				require.Equal(t, u2.Age, u.Age)
			case u3.ID:
				foundU3 = true
				require.Equal(t, u3.Name, u.Name)
				require.Equal(t, u3.Email, u.Email)
				require.Equal(t, u3.Age, u.Age)
			}
		}
		require.True(t, foundU1_2, "should find u1")
		require.True(t, foundU2_2, "should find u2")
		require.True(t, foundU3, "should find u3")
	})

	t.Run("RawQuery", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)
		users := make([]*TestUser, 0)

		// Test RawQuery with nil query: age > 18
		// Should return u2 (age=19) and u3 (age=20)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age > ?",
				RawQueryArgs: []any{18},
			}).
			List(&users))
		require.Len(t, users, 2)
		var foundU2, foundU3 bool
		for _, u := range users {
			if u.ID == u2.ID {
				foundU2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Age, u.Age)
				require.Equal(t, u2.Email, u.Email)
				require.Equal(t, u2.IsActive, u.IsActive)
			}
			if u.ID == u3.ID {
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
		require.True(t, foundU2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test RawQuery with empty struct query: age >= 19
		// Should return u2 (age=19) and u3 (age=20)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{}, types.QueryConfig{
				RawQuery:     "age >= ?",
				RawQueryArgs: []any{19},
			}).
			List(&users))
		require.Len(t, users, 2)
		foundU2, foundU3 = false, false
		for _, u := range users {
			if u.ID == u2.ID {
				foundU2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Age, u.Age)
				require.Equal(t, u2.Email, u.Email)
			}
			if u.ID == u3.ID {
				foundU3 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u3.Name, u.Name)
				require.Equal(t, u3.Age, u.Age)
				require.Equal(t, u3.Email, u.Email)
			}
		}
		require.True(t, foundU2, "should find u2")
		require.True(t, foundU3, "should find u3")

		// Test RawQuery with multiple conditions: age BETWEEN ? AND ?
		// Should return u2 (age=19)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age BETWEEN ? AND ?",
				RawQueryArgs: []any{19, 19},
			}).
			List(&users))
		require.Len(t, users, 1)
		require.NotEmpty(t, users[0].ID)
		require.NotEmpty(t, users[0].CreatedAt)
		require.NotEmpty(t, users[0].UpdatedAt)
		require.Equal(t, u2.ID, users[0].ID)
		require.Equal(t, u2.Name, users[0].Name)
		require.Equal(t, u2.Age, users[0].Age)
		require.Equal(t, u2.Email, users[0].Email)
		require.Equal(t, u2.IsActive, users[0].IsActive)

		// Test RawQuery with string condition: name = ?
		// Should return u1 (name="user1")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "name = ?",
				RawQueryArgs: []any{u1.Name},
			}).
			List(&users))
		require.Len(t, users, 1)
		require.NotEmpty(t, users[0].ID)
		require.NotEmpty(t, users[0].CreatedAt)
		require.NotEmpty(t, users[0].UpdatedAt)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)
		require.Equal(t, u1.Email, users[0].Email)
		require.Equal(t, u1.IsActive, users[0].IsActive)

		// Test RawQuery with OR condition: name = ? OR age = ?
		// Should return u1 (name="user1") and u2 (age=19)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "name = ? OR age = ?",
				RawQueryArgs: []any{u1.Name, u2.Age},
			}).
			List(&users))
		require.Len(t, users, 2)
		foundU1, foundU2 := false, false
		for _, u := range users {
			if u.ID == u1.ID {
				foundU1 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Age, u.Age)
				require.Equal(t, u1.Email, u.Email)
				require.Equal(t, u1.IsActive, u.IsActive)
			}
			if u.ID == u2.ID {
				foundU2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u2.Name, u.Name)
				require.Equal(t, u2.Age, u.Age)
				require.Equal(t, u2.Email, u.Email)
				require.Equal(t, u2.IsActive, u.IsActive)
			}
		}
		require.True(t, foundU1, "should find u1")
		require.True(t, foundU2, "should find u2")

		// Test RawQuery with IN clause: age IN (?)
		// Should return u1 (age=18) and u3 (age=20)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age IN (?)",
				RawQueryArgs: []any{[]int{18, 20}},
			}).
			List(&users))
		require.Len(t, users, 2)
		var foundU1_2, foundU3_2 bool
		for _, u := range users {
			if u.ID == u1.ID {
				foundU1_2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u1.Name, u.Name)
				require.Equal(t, u1.Age, u.Age)
				require.Equal(t, u1.Email, u.Email)
				require.Equal(t, u1.IsActive, u.IsActive)
			}
			if u.ID == u3.ID {
				foundU3_2 = true
				require.NotEmpty(t, u.ID)
				require.NotEmpty(t, u.CreatedAt)
				require.NotEmpty(t, u.UpdatedAt)
				require.Equal(t, u3.Name, u.Name)
				require.Equal(t, u3.Age, u.Age)
				require.Equal(t, u3.Email, u.Email)
				require.Equal(t, u3.IsActive, u.IsActive)
			}
		}
		require.True(t, foundU1_2, "should find u1")
		require.True(t, foundU3_2, "should find u3")

		// Test RawQuery with AND condition: name = ? AND age = ?
		// Should return u1 (name="user1" AND age=18)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "name = ? AND age = ?",
				RawQueryArgs: []any{u1.Name, u1.Age},
			}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)

		// Test RawQuery with AND condition that matches no records: name = ? AND age = ?
		// Should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "name = ? AND age = ?",
				RawQueryArgs: []any{u1.Name, u2.Age},
			}).
			List(&users))
		require.Empty(t, users)

		// Test RawQuery with no matching condition: age > 100
		// Should return 0 records
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age > ?",
				RawQueryArgs: []any{100},
			}).
			List(&users))
		require.Empty(t, users)

		// Test RawQuery with empty RawQueryArgs (should work when query has no placeholders)
		// Query: age = 18 (hardcoded value, no placeholders)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age = 18",
				RawQueryArgs: nil,
			}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Age, users[0].Age)

		// Test RawQuery with empty RawQueryArgs slice (should work when query has no placeholders)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "age = 18",
				RawQueryArgs: []any{},
			}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Age, users[0].Age)

		// Test RawQuery combined with model fields: both conditions are applied with AND logic
		// RawQuery: age > 18, Query: Name="user1"
		// Should return 0 records (no user with name="user1" AND age > 18, since u1 has age=18)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name}, types.QueryConfig{
				RawQuery:     "age > ?",
				RawQueryArgs: []any{18},
			}).
			List(&users))
		require.Empty(t, users, "RawQuery and model fields should be combined with AND logic")

		// Test RawQuery combined with model fields: both conditions match
		// RawQuery: age >= 18, Query: Name="user1"
		// Should return u1 (name="user1" AND age >= 18)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u1.Name}, types.QueryConfig{
				RawQuery:     "age >= ?",
				RawQueryArgs: []any{18},
			}).
			List(&users))
		require.Len(t, users, 1, "RawQuery and model fields should be combined with AND logic")
		require.Equal(t, u1.ID, users[0].ID)
		require.Equal(t, u1.Name, users[0].Name)
		require.Equal(t, u1.Age, users[0].Age)

		// Test RawQuery combined with model fields: multiple model fields
		// RawQuery: age > 18, Query: Name="user2", Email="user2@example.com"
		// Should return u2 (name="user2" AND email="user2@example.com" AND age > 18)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(&TestUser{Name: u2.Name, Email: u2.Email}, types.QueryConfig{
				RawQuery:     "age > ?",
				RawQueryArgs: []any{18},
			}).
			List(&users))
		require.Len(t, users, 1, "RawQuery and multiple model fields should be combined with AND logic")
		require.Equal(t, u2.ID, users[0].ID)
		require.Equal(t, u2.Name, users[0].Name)
		require.Equal(t, u2.Age, users[0].Age)
		require.Equal(t, u2.Email, users[0].Email)

		// Test RawQuery with complex condition: (name = ? OR email = ?) AND age >= ?
		// Should return u2 (email="user2@example.com" AND age=19)
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "(name = ? OR email = ?) AND age >= ?",
				RawQueryArgs: []any{u2.Name, u2.Email, 19},
			}).
			List(&users))
		require.Len(t, users, 1)
		require.Equal(t, u2.ID, users[0].ID)
		require.Equal(t, u2.Name, users[0].Name)
		require.Equal(t, u2.Age, users[0].Age)
		require.Equal(t, u2.Email, users[0].Email)

		// Test RawQuery with LIKE pattern: name LIKE ?
		// Should return all 3 records (all names contain "user")
		users = make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).
			WithQuery(nil, types.QueryConfig{
				RawQuery:     "name LIKE ?",
				RawQueryArgs: []any{"%user%"},
			}).
			List(&users))
		require.Len(t, users, 3)
		var foundU1_3, foundU2_3, foundU3_3 bool
		for _, u := range users {
			switch u.ID {
			case u1.ID:
				foundU1_3 = true
			case u2.ID:
				foundU2_3 = true
			case u3.ID:
				foundU3_3 = true
			}
		}
		require.True(t, foundU1_3, "should find u1")
		require.True(t, foundU2_3, "should find u2")
		require.True(t, foundU3_3, "should find u3")
	})
}
