package database_test

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestDatabaseWithQuery(t *testing.T) {
	t.Run("ConditionsRenderInColumnOrder", func(t *testing.T) {
		// Every non-zero field of the model becomes one condition, rendered in
		// column name order: the same query is the same statement on every
		// run, whatever order a map would hand the columns out in.
		var first string
		var args []any
		for range 20 {
			statements := make([]types.SQLStatement, 0)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).
				WithDryRun(&statements).
				WithQuery(&TestUser{Name: "name", Email: "email", Age: 18}).
				List(&users))
			require.Len(t, statements, 1)
			if first == "" {
				first = statements[0].Query
			}
			require.Equal(t, first, statements[0].Query)
			args = statements[0].Args
		}
		age := strings.Index(first, quoteIdent("age"))
		email := strings.Index(first, quoteIdent("email"))
		name := strings.Index(first, quoteIdent("name"))
		require.True(t, age >= 0 && age < email && email < name, "conditions must follow column name order: %s", first)
		require.Equal(t, []any{18, "email", "name"}, args, "every value binds as its field holds it")
	})

	t.Run("FieldsNameTheirOwnColumns", func(t *testing.T) {
		// Each field is a condition on its own column: the two embedded
		// copies of TestPart carry their prefixes, and the top-level Label is
		// neither of them, though all three share a Go name.
		ctx := context.Background()
		require.NoError(t, database.DB().AutoMigrate(&TestPairedRecord{}))
		t.Cleanup(func() {
			require.NoError(t, database.DB().Migrator().DropTable(&TestPairedRecord{}))
		})
		first := &TestPairedRecord{Label: "l1", First: TestPart{Label: "f1"}, Second: TestPart{Label: "s1"}}
		second := &TestPairedRecord{Label: "f1", First: TestPart{Label: "s1"}, Second: TestPart{Label: "l1"}}
		require.NoError(t, database.Database[*TestPairedRecord](ctx).Create(first, second))

		for _, query := range []*TestPairedRecord{
			{Label: "l1"},
			{First: TestPart{Label: "f1"}},
			{Second: TestPart{Label: "s1"}},
		} {
			rows := make([]*TestPairedRecord, 0)
			require.NoError(t, database.Database[*TestPairedRecord](ctx).WithQuery(query).List(&rows))
			require.Len(t, rows, 1)
			require.Equal(t, first.ID, rows[0].ID)
		}

		// A delete by conditions names the same column.
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Database[*TestPairedRecord](ctx).
			WithDryRun(&statements).
			WithQuery(&TestPairedRecord{First: TestPart{Label: "f1"}}).
			Delete())
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query, quoteIdent("first_label")+" = ")
	})

	t.Run("ValuersBindTheirStoredForm", func(t *testing.T) {
		// A column whose type implements driver.Valuer stores what Value
		// produces, and a condition on it binds that as well, the way the
		// operator filters bind their value: both find the row.
		ctx := context.Background()
		require.NoError(t, database.DB().AutoMigrate(&TestPairedRecord{}))
		t.Cleanup(func() {
			require.NoError(t, database.DB().Migrator().DropTable(&TestPairedRecord{}))
		})
		row := &TestPairedRecord{Label: "coded", Code: "abc"}
		require.NoError(t, database.Database[*TestPairedRecord](ctx).Create(row))

		rows := make([]*TestPairedRecord, 0)
		require.NoError(t, database.Database[*TestPairedRecord](ctx).WithQuery(&TestPairedRecord{Code: "abc"}).List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, row.ID, rows[0].ID)

		rows = make([]*TestPairedRecord, 0)
		require.NoError(t, database.Database[*TestPairedRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{types.FilterEq("code", TestTaggedCode("abc"))}}).
			List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, row.ID, rows[0].ID)
	})

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

	t.Run("JSONColumnsFailClosed", func(t *testing.T) {
		ctx := context.Background()
		t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_scored_records").Error })
		row := &TestScoredRecord{
			Name:    "scored",
			Scores:  datatypes.NewJSONSlice([]int{1, 2}),
			Weights: datatypes.NewJSONSlice([]float64{0.5}),
			Meta:    datatypes.JSONMap{"k": "v"},
			Extra:   datatypes.NewJSONType(map[string]string{"k": "v"}),
		}
		require.NoError(t, database.Database[*TestScoredRecord](ctx).Create(row))

		rows := make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(&TestScoredRecord{Name: "scored"}).List(&rows))
		require.Len(t, rows, 1)

		// An exact match on a JSON document cannot compare, so a condition on
		// a number array, a map or a typed document answers no rows, as one
		// on a string array does, instead of being dropped and leaving the
		// name alone to match.
		for _, query := range []*TestScoredRecord{
			{Name: "scored", Scores: datatypes.NewJSONSlice([]int{1, 2})},
			{Name: "scored", Weights: datatypes.NewJSONSlice([]float64{0.5})},
			{Name: "scored", Meta: datatypes.JSONMap{"k": "v"}},
			{Name: "scored", Extra: datatypes.NewJSONType(map[string]string{"k": "v"})},
		} {
			rows = make([]*TestScoredRecord, 0)
			require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(query).List(&rows))
			require.Empty(t, rows)
		}

		// A string its type tag declares a JSON column holds a document as
		// much as a JSON type does.
		t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_typed_records").Error })
		require.NoError(t, database.Database[*TestTypedRecord](ctx).Create(&TestTypedRecord{Label: "typed", Payload: `{"code":"alpha"}`}))
		typed := make([]*TestTypedRecord, 0)
		require.NoError(t, database.Database[*TestTypedRecord](ctx).WithQuery(&TestTypedRecord{Label: "typed", Payload: `{"code":"alpha"}`}).List(&typed))
		require.Empty(t, typed)
	})

	t.Run("NullableValuesMatchByTheirValue", func(t *testing.T) {
		ctx := context.Background()
		t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_scored_records").Error })
		row := &TestScoredRecord{Name: "graded", Grade: sql.NullString{String: "a", Valid: true}}
		require.NoError(t, database.Database[*TestScoredRecord](ctx).Create(row))

		rows := make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(&TestScoredRecord{Name: "graded", Grade: sql.NullString{String: "a", Valid: true}}).List(&rows))
		require.Len(t, rows, 1, "a nullable value is a condition on the value it carries")
		require.Equal(t, row.ID, rows[0].ID)

		rows = make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(&TestScoredRecord{Name: "graded", Grade: sql.NullString{String: "b", Valid: true}}).List(&rows))
		require.Empty(t, rows)
	})

	t.Run("InstantsAreConditions", func(t *testing.T) {
		ctx := context.Background()
		cleanupAggregateData()
		t.Cleanup(cleanupAggregateData)
		first, second, closed := seedTime(3, 1, 8), seedTime(3, 2, 9), seedTime(3, 3, 10)
		rows := []*TestAggregateRecord{
			{Category: "timed", OccurredAt: first, ClosedAt: &closed},
			{Category: "timed", OccurredAt: second},
		}
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).Create(rows...))

		listed := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).WithQuery(&TestAggregateRecord{Category: "timed", OccurredAt: first}).List(&listed))
		require.Len(t, listed, 1, "an instant on the query model is a condition, not a field the query ignores")
		require.Equal(t, rows[0].ID, listed[0].ID)

		// The same instant spelled in another offset names the same row.
		listed = make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).WithQuery(&TestAggregateRecord{Category: "timed", OccurredAt: first.In(time.FixedZone("", 8*3600))}).List(&listed))
		require.Len(t, listed, 1)
		require.Equal(t, rows[0].ID, listed[0].ID)

		listed = make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).WithQuery(&TestAggregateRecord{Category: "timed", ClosedAt: &closed}).List(&listed))
		require.Len(t, listed, 1, "an optional instant behind a pointer is a condition too")
		require.Equal(t, rows[0].ID, listed[0].ID)
	})

	t.Run("DaysMatchTheUTCDay", func(t *testing.T) {
		ctx := context.Background()
		cleanupAggregateData()
		t.Cleanup(cleanupAggregateData)
		first, second := seedTime(1, 10, 0), seedTime(1, 11, 0)
		rows := []*TestDatedRecord{
			{Label: "dated-first", Day: datatypes.Date(first), Due: &first},
			{Label: "dated-second", Day: datatypes.Date(second), Due: &second},
		}
		require.NoError(t, database.Database[*TestDatedRecord](ctx).Create(rows...))

		// A day on the query model is matched by the UTC day it names,
		// whatever time of day and offset the value carries, declared a date
		// by its type or by its type tag alike: 23:30 at +08:00 on the tenth
		// is the tenth in UTC.
		sent := time.Date(2024, 1, 10, 23, 30, 0, 0, time.FixedZone("", 8*3600))
		for _, query := range []*TestDatedRecord{{Day: datatypes.Date(sent)}, {Due: &sent}} {
			listed := make([]*TestDatedRecord, 0)
			require.NoError(t, database.Database[*TestDatedRecord](ctx).WithQuery(query).List(&listed))
			require.Len(t, listed, 1, "a day on the query model is a condition, not a field the query ignores")
			require.Equal(t, rows[0].ID, listed[0].ID)
		}
	})

	t.Run("ByteSliceMatchesByteForByte", func(t *testing.T) {
		ctx := context.Background()
		t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_scored_records").Error })
		row := &TestScoredRecord{Name: "digested", Digest: []byte{0x00, 0xff, 0x10}}
		require.NoError(t, database.Database[*TestScoredRecord](ctx).Create(row))

		rows := make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(&TestScoredRecord{Name: "digested", Digest: []byte{0x00, 0xff, 0x10}}).List(&rows))
		require.Len(t, rows, 1, "the same bytes match")
		require.Equal(t, row.ID, rows[0].ID)

		rows = make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).WithQuery(&TestScoredRecord{Name: "digested", Digest: []byte{0x00, 0xff}}).List(&rows))
		require.Empty(t, rows, "other bytes do not")
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
		// Empty strings are zero values and are skipped, so this reaches the same
		// empty-query check as the empty struct above
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
		// The empty strings are skipped like any zero value, so AllowEmpty lifts
		// the same empty-query check as for the empty struct
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
