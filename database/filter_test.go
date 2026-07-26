package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// Tests for the predicate engine in filter.go: the operator filters, their
// grouping and nesting, and the fail-closed rules. WithQuery's own behavior --
// how a model value becomes conditions -- is covered in query_test.go.

func TestDatabaseFilterGroups(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	list := func(t *testing.T, filters ...types.Filter) []string {
		t.Helper()
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: filters}).
			WithOrder(types.Asc("id")).
			List(&users))
		ids := make([]string, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		return ids
	}

	t.Run("OrGroupMatchesAnyChild", func(t *testing.T) {
		require.Equal(t, []string{u1.ID, u2.ID}, list(t, types.FilterOr(
			types.FilterEq("name", u1.Name),
			types.FilterEq("age", u2.Age),
		)))
	})

	// The group is one element of an AND list, so a mandatory condition
	// next to it can never be absorbed into the alternatives. This is the
	// exact shape the removed QueryOptions.Or switch got wrong.
	t.Run("GroupStaysAndCombinedWithOtherFilters", func(t *testing.T) {
		require.Empty(t, list(
			t,
			types.FilterEq("name", u1.Name),
			types.FilterOr(
				types.FilterEq("age", u2.Age),
				types.FilterEq("age", u3.Age),
			),
		), "the mandatory condition must not be OR-ed away")
	})

	t.Run("RawQueryStaysAndCombinedWithGroup", func(t *testing.T) {
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{
				AllowEmpty:   true,
				RawQuery:     "name = ?",
				RawQueryArgs: []any{u1.Name},
				Filters: []types.Filter{types.FilterOr(
					types.FilterEq("age", u2.Age),
					types.FilterEq("age", u3.Age),
				)},
			}).
			List(&users))
		require.Empty(t, users, "RawQuery is a mandatory condition too")
	})

	t.Run("AndGroupsNestedInOrGroup", func(t *testing.T) {
		require.Equal(t, []string{u1.ID, u3.ID}, list(t, types.FilterOr(
			types.FilterAnd(
				types.FilterEq("name", u1.Name),
				types.FilterEq("age", u1.Age),
			),
			types.FilterAnd(
				types.FilterEq("name", u3.Name),
				types.FilterEq("age", u3.Age),
			),
		)), "(a AND b) OR (c AND d) must match only the fully matching rows")

		require.Empty(t, list(t, types.FilterOr(
			types.FilterAnd(
				types.FilterEq("name", u1.Name),
				types.FilterEq("age", u3.Age),
			),
			types.FilterAnd(
				types.FilterEq("name", u3.Name),
				types.FilterEq("age", u1.Age),
			),
		)), "a child group matches only when all of its own conditions hold")
	})

	t.Run("ThreeLevelNesting", func(t *testing.T) {
		require.Equal(t, []string{u1.ID, u2.ID}, list(t, types.FilterOr(
			types.FilterEq("name", u1.Name),
			types.FilterAnd(
				types.FilterEq("email", u2.Email),
				types.FilterOr(
					types.FilterEq("age", u2.Age),
					types.FilterEq("age", u3.Age),
				),
			),
		)))
	})

	t.Run("TopLevelAndGroupEqualsFlatFilters", func(t *testing.T) {
		grouped := list(t, types.FilterAnd(
			types.FilterEq("name", u1.Name),
			types.FilterEq("age", u1.Age),
		))
		flat := list(
			t,
			types.FilterEq("name", u1.Name),
			types.FilterEq("age", u1.Age),
		)
		require.Equal(t, flat, grouped)
		require.Equal(t, []string{u1.ID}, grouped)
	})

	// An empty group is a caller bug. Answering it with the logical
	// identity (TRUE for AND) would widen the result set, so both group
	// operators fail closed instead.
	t.Run("MalformedGroupsFailClosed", func(t *testing.T) {
		require.Empty(t, list(t, types.FilterOr()), "empty OR group")
		require.Empty(t, list(t, types.FilterAnd()), "empty AND group")
		require.Empty(t, list(t, types.Filter{Op: types.FilterOpOr, Value: "oops"}),
			"a group value that is not a filter list")
		require.Empty(t, list(t, types.FilterOr(types.FilterEq("", "x"))),
			"a child with an empty column")
	})
}

func TestDatabaseFilters(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)
	users := make([]*TestUser, 0)

	// Test gt with nil query: age > 18 should return u2 (age=19) and u3 (age=20).
	// Conditions alone count as real conditions, so the nil query is not
	// blocked by the empty-query safety check.
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpGt, Value: "18"},
			},
		}).
		List(&users))
	require.Len(t, users, 2)
	var foundU2, foundU3 bool
	for _, u := range users {
		switch u.ID {
		case u2.ID:
			foundU2 = true
		case u3.ID:
			foundU3 = true
		}
	}
	require.True(t, foundU2, "should find u2")
	require.True(t, foundU3, "should find u3")

	// Test AND combination with an exact model filter:
	// Name="user1" AND age >= 18 should return u1.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(&TestUser{Name: u1.Name}, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpGte, Value: "18"},
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)

	// Name="user1" AND age > 18 should return 0 records (u1 has age=18).
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(&TestUser{Name: u1.Name}, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpGt, Value: "18"},
			},
		}).
		List(&users))
	require.Empty(t, users, "conditions combine with exact filters using AND logic")

	// Test eq: age = 19 should return u2.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpEq, Value: "19"},
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u2.ID, users[0].ID)

	// Test ne: age <> 19 should return u1 and u3.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpNe, Value: "19"},
			},
		}).
		List(&users))
	require.Len(t, users, 2)
	var foundU1 bool
	foundU3 = false
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
		case u3.ID:
			foundU3 = true
		}
	}
	require.True(t, foundU1, "should find u1")
	require.True(t, foundU3, "should find u3")

	// Test lt and lte: age < 19 should return u1; age <= 19 should return u1 and u2.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpLt, Value: "19"},
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpLte, Value: "19"},
			},
		}).
		List(&users))
	require.Len(t, users, 2)

	// Test like: the value is wrapped with wildcards, so email like "@example"
	// matches every user by substring.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "email", Op: types.FilterOpLike, Value: "@example"},
			},
		}).
		List(&users))
	require.Len(t, users, 3, "like should match substrings")

	// Test notlike: name not like "1" should return u2 and u3.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpNotLike, Value: "1"},
			},
		}).
		List(&users))
	require.Len(t, users, 2, "notlike should exclude substring matches")

	// Test in and notin: comma-separated values split into a set.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpIn, Value: []string{"18", "20"}},
			},
		}).
		List(&users))
	require.Len(t, users, 2)
	foundU1, foundU3 = false, false
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
		case u3.ID:
			foundU3 = true
		}
	}
	require.True(t, foundU1, "should find u1")
	require.True(t, foundU3, "should find u3")

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpNotIn, Value: []string{"user1", "user2"}},
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u3.ID, users[0].ID)

	// Test startswith is anchored at the beginning: "user" prefixes every
	// name, while "ser" only appears inside and must not match.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpStartsWith, Value: "user"},
			},
		}).
		List(&users))
	require.Len(t, users, 3)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpStartsWith, Value: "ser"},
			},
		}).
		List(&users))
	require.Empty(t, users, "startswith must anchor at the beginning of the value")

	// Test endswith is anchored at the end.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "email", Op: types.FilterOpEndsWith, Value: "1@example.com"},
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "email", Op: types.FilterOpEndsWith, Value: "user"},
			},
		}).
		List(&users))
	require.Empty(t, users, "endswith must anchor at the end of the value")

	// Test isnull: fixtures leave is_active NULL; one extra record sets it.
	// (remark is unusable here: the TestUser CreateBefore hook fills it on
	// every create, so it is never NULL.)
	active := true
	flaggedUser := &TestUser{Name: "user4", Email: "user4@example.com", Age: 21, IsActive: &active, Base: model.Base{ID: "u4"}}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(flaggedUser))

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "is_active", Op: types.FilterOpIsNull, Value: true},
			},
		}).
		List(&users))
	require.Len(t, users, 3, "isnull=1 must select records whose column IS NULL")

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "is_active", Op: types.FilterOpIsNull, Value: false},
			},
		}).
		List(&users))
	require.Len(t, users, 1, "isnull=0 must select records whose column IS NOT NULL")
	require.Equal(t, flaggedUser.ID, users[0].ID)

	// Test LIKE metacharacter escaping: the value is a literal, not a
	// pattern. An unescaped "user_" would match every "userX" via the
	// "_" single-character wildcard; escaped it only matches the record
	// whose name literally contains "user_".
	underscoreUser := &TestUser{Name: "user_x", Email: "user.x@example.com", Age: 22, Base: model.Base{ID: "u5"}}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(underscoreUser))

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpLike, Value: "user_"},
			},
		}).
		List(&users))
	require.Len(t, users, 1, "LIKE metacharacters in the value must be escaped")
	require.Equal(t, underscoreUser.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpStartsWith, Value: "user_"},
			},
		}).
		List(&users))
	require.Len(t, users, 1, "startswith must escape LIKE metacharacters too")
	require.Equal(t, underscoreUser.ID, users[0].ID)

	// Test the service-only regex operators: they are not reachable from
	// URL parsing but service code can pass them through Filters.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpRegex, Value: "^user[12]$"},
			},
		}).
		List(&users))
	require.Len(t, users, 2)
	foundU1, foundU2 = false, false
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
		case u2.ID:
			foundU2 = true
		}
	}
	require.True(t, foundU1, "should find u1")
	require.True(t, foundU2, "should find u2")

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpNotRegex, Value: "^user[0-9]$"},
			},
		}).
		List(&users))
	require.Len(t, users, 1, "only the underscored name escapes the pattern")
	require.Equal(t, underscoreUser.ID, users[0].ID)

	// Test the service-only jsoncontains operator on a JSON array column.
	addrUser := &TestUser{Name: "user6", Email: "user6@example.com", Age: 23, Addr: datatypes.NewJSONSlice([]string{"alpha", "beta"}), Base: model.Base{ID: "u6"}}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(addrUser))

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "addr", Op: types.FilterOpJSONContains, Value: "alpha"},
			},
		}).
		List(&users))
	require.Len(t, users, 1, "jsoncontains must match JSON array membership")
	require.Equal(t, addrUser.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "addr", Op: types.FilterOpJSONContains, Value: "gamma"},
			},
		}).
		List(&users))
	require.Empty(t, users, "jsoncontains must not match absent members")

	// Test fail-closed behavior: an unknown operator or an empty column adds
	// "1 = 0" instead of being dropped, so the query returns no records.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOp("bogus"), Value: "1"},
			},
		}).
		List(&users))
	require.Empty(t, users, "unknown operator must fail closed")

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "", Op: types.FilterOpEq, Value: "1"},
			},
		}).
		List(&users))
	require.Empty(t, users, "empty column must fail closed")
}

func TestDatabaseTypedFilterValues(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// A slice value binds IN directly, so members keep commas literal.
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIn, Value: []string{u1.Name, u3.Name}}},
		}).
		List(&users))
	require.Len(t, users, 2, "slice values must bind IN directly")

	// A slice of a named string type binds the same way as []string, so
	// enum-typed values need no per-member conversion.
	type sampleStatus string
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIn, Value: []sampleStatus{sampleStatus(u1.Name), sampleStatus(u3.Name)}}},
		}).
		List(&users))
	require.Len(t, users, 2, "named string type slices must bind like []string")

	// A nil slice, which a variadic Column.In() call with no arguments
	// produces, must behave like an empty one.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIn, Value: []string(nil)}},
		}).
		List(&users))
	require.Empty(t, users, "a nil slice must match nothing")

	// An empty slice matches nothing instead of widening the result set.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIn, Value: []string{}}},
		}).
		List(&users))
	require.Empty(t, users, "empty slice must match nothing")

	// In no longer splits comma strings: a string value fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIn, Value: u1.Name + "," + u2.Name}},
		}).
		List(&users))
	require.Empty(t, users, "string value on In must fail closed")

	// A scalar comparison rejects slice values and fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "age", Op: types.FilterOpGt, Value: []int{18}}},
		}).
		List(&users))
	require.Empty(t, users, "slice value on a scalar operator must fail closed")

	// A nil value fails closed on every operator family.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "age", Op: types.FilterOpEq, Value: nil}},
		}).
		List(&users))
	require.Empty(t, users, "nil value must fail closed")

	// IsNull carries a bool: false means IS NOT NULL and matches every row
	// of a NOT NULL column, while the legacy "1" string now fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIsNull, Value: false}},
		}).
		List(&users))
	require.Len(t, users, 3, "IsNull false must mean IS NOT NULL")
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{{Column: "name", Op: types.FilterOpIsNull, Value: "1"}},
		}).
		List(&users))
	require.Empty(t, users, "string value on IsNull must fail closed")

	// Typed Go values bind directly on comparison operators.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "age", Op: types.FilterOpGt, Value: 18},
				{Column: "created_at", Op: types.FilterOpGte, Value: time.Now().Add(-time.Hour)},
			},
		}).
		List(&users))
	require.Len(t, users, 2, "typed scalar values must bind directly")
}

// TestFilterOrSingleChild is the regression test for a group that collapses to
// one alternative. gorm reads a one-element OrConditions as an OR *connector*,
// so handing it that shape joins the group to the preceding condition with OR
// and turns a mandatory sibling into an alternative -- a silent widening that
// reads every row the mandatory condition was there to hide.
func TestFilterOrSingleChild(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	list := func(t *testing.T, filters ...types.Filter) []string {
		t.Helper()
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: filters}).
			WithOrder(types.Asc("id")).
			List(&users))
		ids := make([]string, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		return ids
	}

	// The two conditions must match different rows, otherwise AND and OR
	// produce the same result and the assertion cannot tell them apart: the
	// mandatory condition selects u1, the single alternative selects u2.
	t.Run("MandatorySiblingSurvives", func(t *testing.T) {
		require.Empty(t, list(
			t,
			types.FilterEq("age", u1.Age),
			types.FilterOr(types.FilterEq("name", u2.Name)),
		), "no row satisfies both, so a degraded group would show up as u1 and u2")
	})

	t.Run("EquivalentToTheBareCondition", func(t *testing.T) {
		require.Equal(t,
			list(t, types.FilterEq("name", u2.Name)),
			list(t, types.FilterOr(types.FilterEq("name", u2.Name))),
			"OR over one alternative is that alternative")
	})

	t.Run("NestedSingleChildGroups", func(t *testing.T) {
		require.Empty(t, list(
			t,
			types.FilterEq("age", u1.Age),
			types.FilterOr(types.FilterAnd(types.FilterOr(types.FilterEq("name", u2.Name)))),
		), "collapsing through several levels must still leave an AND")
	})

	t.Run("MultipleAlternativesStillGrouped", func(t *testing.T) {
		require.Equal(t, []string{u1.ID}, list(
			t,
			types.FilterEq("age", u1.Age),
			types.FilterOr(
				types.FilterEq("name", u1.Name),
				types.FilterEq("name", u2.Name),
			),
		), "u2 matches an alternative but fails the mandatory condition")
	})
}
