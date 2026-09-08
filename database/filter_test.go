package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
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

	// FilterFalse is the predicate that matches nothing on purpose, the way a
	// permission hook denies every row. It is a real condition: it disables
	// the empty-query safety check like any filter, and next to a group it
	// stays a mandatory AND term instead of being OR-ed away.
	t.Run("FalseMatchesNothing", func(t *testing.T) {
		require.Empty(t, list(t, types.FilterFalse()))
		require.Empty(t, list(t, types.FilterFalse(), types.FilterOr(
			types.FilterEq("age", u2.Age),
			types.FilterEq("age", u3.Age),
		)), "the false predicate is a mandatory condition too")
	})

	// Inside an OR group the false predicate is one alternative that never
	// holds, so the other alternatives decide alone.
	t.Run("FalseInsideOrGroupIsInert", func(t *testing.T) {
		require.Equal(t, []string{u2.ID}, list(t, types.FilterOr(
			types.FilterEq("age", u2.Age),
			types.FilterFalse(),
		)))
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
	flaggedUser := &TestUser{Name: "user4", Email: "user4@example.com", Age: 21, IsActive: &active, ID: "u4"}
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
	underscoreUser := &TestUser{Name: "user_x", Email: "user.x@example.com", Age: 22, ID: "u5"}
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

	// An invalid pattern fails the query on every dialect instead of being
	// silently dropped: MySQL rejects it in its regex engine, sqlite in the
	// Go implementation the framework registers, postgres in its own.
	users = make([]*TestUser, 0)
	require.Error(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				{Column: "name", Op: types.FilterOpRegex, Value: "(unclosed"},
			},
		}).
		List(&users))

	// Test the service-only jsoncontains operator on a JSON array column.
	addrUser := &TestUser{Name: "user6", Email: "user6@example.com", Age: 23, Addr: datatypes.NewJSONSlice([]string{"alpha", "beta"}), ID: "u6"}
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

// TestDatabaseFiltersOnTimeColumns pins the runtime behavior of time
// comparisons across dialects: a bound travels either as the UTC wall clock
// in types.FilterTimeLayout (the URL parser's canonical form) or as a
// time.Time, and both must select the same rows however the dialect stores
// time. This is the filter-side counterpart of the cursor paging test and
// exercises the time normalization sqlite needs for its text storage.
func TestDatabaseFiltersOnTimeColumns(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	listIDs := func(t *testing.T, f types.Filter) []string {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](context.Background()).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{f}}).
			WithOrder(types.Asc("id")).
			List(&records))
		ids := make([]string, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.ID)
		}
		return ids
	}

	boundary := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	t.Run("CanonicalStringBound", func(t *testing.T) {
		require.Equal(t, []string{"a4", "a5", "a6"},
			listIDs(t, types.FilterGte("occurred_at", boundary.Format(types.FilterTimeLayout))))
	})

	t.Run("TimeValueBound", func(t *testing.T) {
		require.Equal(t, []string{"a1", "a2", "a3"},
			listIDs(t, types.FilterLt("occurred_at", boundary)))
	})

	t.Run("EqualityOnAnExactInstant", func(t *testing.T) {
		instant := time.Date(2024, 1, 11, 8, 0, 0, 0, time.UTC)
		require.Equal(t, []string{"a3"}, listIDs(t, types.FilterEq("occurred_at", instant)))
	})
}

func TestFilterOfAnotherTableFailsTheChain(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	// A column reference carries its table, and only service code writes
	// one: a filter of a table the query does not read is a wrong-model read,
	// not client input, so it fails the chain the way WithSelect refuses the
	// same reference, rather than failing closed to an empty page that reads
	// like "no data today". The name alone could have matched a column of
	// the queried model by coincidence and filtered the wrong table as valid
	// SQL.
	users := make([]*TestUser, 0)
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{TestAggregateRecordCols.Category.Eq("alpha")}}).
		List(&users), database.ErrColumnTable)
	// Inside a group as well: the groups are walked, a subquery's filters
	// are the subquery's own.
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
			types.FilterOr(colName.Eq("user1"), types.FilterAnd(TestAggregateRecordCols.Status.Eq("done"))),
		}}).
		List(&users), database.ErrColumnTable)
	count := 0
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{TestAggregateRecordCols.Category.Eq("alpha")}}).
		Count(&count), database.ErrColumnTable)
	// Inside a subquery a filter names the related model's table; a third
	// table is as foreign there as at the top level, and only service code
	// could have written it.
	records := make([]*TestAggregateRecord, 0)
	require.ErrorIs(t, database.Database[*TestAggregateRecord](context.Background()).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
			types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestPaymentCols.Account.Eq("acme")),
		}}).
		List(&records), database.ErrColumnTable)
}

// The semi-join tests below cover FilterExists and FilterNotExists over the
// aggregate fixture: the records and the tags that point at them, described at
// aggregateSeed and tagSeed in fixture_test.go.

func TestFilterExists(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	vip := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))

	// The same filter serves List and Select: it is a Filter operator, not
	// a select feature.
	t.Run("NarrowsList", func(t *testing.T) {
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
			WithOrder(types.Asc("id")).
			List(&records))
		require.Len(t, records, 2)
		require.Equal(t, "a1", records[0].ID)
		require.Equal(t, "a3", records[1].ID)
	})

	t.Run("NarrowsSelect", func(t *testing.T) {
		type row struct {
			Total   int64
			Records int64
		}
		got := row{}
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Sum().As("total"), types.Count().As("records")).
			Where(vip).
			ScanOne(&got))
		require.EqualValues(t, 2, got.Records)
		require.EqualValues(t, 400, got.Total, "a1 is 100 and a3 is 300")
	})

	t.Run("CountsEachRowOnce", func(t *testing.T) {
		// a1 gets a second vip tag. A join would duplicate the row and double
		// its amount; a semi join matches it once.
		require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
			&TestRecordTag{ID: "t4", RecordID: "a1", Label: "vip"},
		))
		type row struct {
			Total   int64
			Records int64
		}
		got := row{}
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Sum().As("total"), types.Count().As("records")).
			Where(vip).
			ScanOne(&got))
		require.EqualValues(t, 2, got.Records, "a second tag must not duplicate the row")
		require.EqualValues(t, 400, got.Total)
	})

	t.Run("HidesSoftDeletedRelatedRows", func(t *testing.T) {
		require.NoError(t, database.Database[*TestRecordTag](ctx).Delete(
			&TestRecordTag{ID: "t2"},
		))
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
			List(&records))
		require.Len(t, records, 1, "a3 loses its only live vip tag")
		require.Equal(t, "a1", records[0].ID)
	})

	// A false predicate inside the subquery is a real condition of that
	// subquery, not a rendering failure: EXISTS then matches no row and NOT
	// EXISTS matches every row, instead of both collapsing to nothing the way
	// an unusable predicate does.
	t.Run("FalseInsideSubquery", func(t *testing.T) {
		none := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), types.FilterFalse())
		all := types.FilterNotExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), types.FilterFalse())
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{none}}).
			List(&records))
		require.Empty(t, records)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{all}}).
			List(&records))
		require.Len(t, records, 6)
	})
}

func TestFilterNotExists(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// Rows with no vip tag at all, plus rows whose tags are not vip.
	noVip := types.FilterNotExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{noVip}}).
		WithOrder(types.Asc("id")).
		List(&records))

	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	require.Equal(t, []string{"a2", "a4", "a5", "a6"}, ids,
		"a4 carries only a bulk tag, so it counts as having no vip tag")
}

func TestFilterExistsCombinesWithOtherFilters(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	vip := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
			vip,
			TestAggregateRecordCols.Status.Eq("failed"),
		}}).
		List(&records))
	require.Len(t, records, 1, "a1 is done and a3 is failed")
	require.Equal(t, "a3", records[0].ID)
}

// TestSelectConditionalOnSubquery covers the combination a report reaches
// for when the measure's own table carries no flag to split on: the split
// lives in a related table, so the CASE predicate is a correlated subquery
// rather than a column comparison.
//
// A join would be the obvious alternative and the wrong one: joining a
// one-to-many child multiplies the outer rows, and the SUM then counts a row
// once per related row instead of once.
func TestSelectConditionalOnSubquery(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	tagged := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))
	untagged := types.FilterNotExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))

	type row struct {
		TaggedAmount   int64
		UntaggedAmount int64
		TaggedRecords  int64
	}
	got := row{}
	require.NoError(t, database.Select[*TestAggregateRecord, row](ctx,
		TestAggregateRecordCols.Amount.Sum().Where(tagged).As("tagged_amount"),
		TestAggregateRecordCols.Amount.Sum().Where(untagged).As("untagged_amount"),
		types.Count().Where(tagged).As("tagged_records"),
	).
		ScanOne(&got))

	// vip tags sit on a1 (100) and a3 (300); the rest carry no vip tag.
	require.EqualValues(t, 400, got.TaggedAmount)
	require.EqualValues(t, 1700, got.UntaggedAmount)
	require.EqualValues(t, 2, got.TaggedRecords)
	require.EqualValues(t, 2100, got.TaggedAmount+got.UntaggedAmount,
		"the two subsets must partition the table exactly once")

	t.Run("SecondRelatedRowDoesNotDoubleCount", func(t *testing.T) {
		require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
			&TestRecordTag{ID: "t9", RecordID: "a1", Label: "vip"},
		))
		again := row{}
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx,
			TestAggregateRecordCols.Amount.Sum().Where(tagged).As("tagged_amount"),
			TestAggregateRecordCols.Amount.Sum().Where(untagged).As("untagged_amount"),
			types.Count().Where(tagged).As("tagged_records"),
		).
			ScanOne(&again))
		require.EqualValues(t, 400, again.TaggedAmount, "a semi join matches a row once")
		require.EqualValues(t, 2, again.TaggedRecords)
	})
}

// TestFilterExistsTableResolution pins the subquery's FROM to the same table
// its correlation qualifies. gorm names the FROM from the struct unless told
// otherwise, and it reads its own TableName method rather than the framework's
// TableName, so a model that overrides only the framework method would be
// selected FROM one table while the correlation referenced another.
func TestFilterExistsTableResolution(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// TestTagAlias resolves to test_record_tags through TableName, while its
	// struct name would make gorm derive test_tag_aliases.
	vip := types.FilterExists[*TestTagAlias](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
		WithOrder(types.Asc("id")).
		List(&records))
	require.Len(t, records, 2)
	require.Equal(t, "a1", records[0].ID)
	require.Equal(t, "a3", records[1].ID)
}

// TestFilterExistsNested pins the inner correlation to the table directly
// enclosing it. Reading the outer chain instead would correlate the grandchild
// against the outermost model, which is valid SQL joined on the wrong table:
// it returns a wrong row set rather than an error.
func TestFilterExistsNested(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	defer func() { _ = database.DB().Exec("DELETE FROM test_tag_notes").Error }()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// A note on t1 only. t1 tags a1, so a1 is the single record reachable
	// through tag -> note.
	require.NoError(t, database.Database[*TestTagNote](ctx).Create(
		&TestTagNote{ID: "n1", TagID: "t1", Body: "checked"},
	))

	noteCols := struct {
		TagID types.Column[string]
		Body  types.Column[string]
	}{
		TagID: types.NewColumn[*TestTagNote, string]("tag_id"),
		Body:  types.NewColumn[*TestTagNote, string]("body"),
	}
	tagIDCol := types.NewColumn[*TestRecordTag, string]("id")

	// EXISTS(tag WHERE tag.record_id = record.id AND EXISTS(note WHERE
	// note.tag_id = tag.id AND note.body = 'checked'))
	//
	// The inner correlation must name the tag table. Naming the record table
	// would compare note.tag_id against record.id, which happens to be a legal
	// comparison of two id columns and silently matches nothing here.
	hasCheckedNote := types.FilterExists[*TestRecordTag](
		TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID),
		types.FilterExists[*TestTagNote](noteCols.TagID.EqCol(tagIDCol), noteCols.Body.Eq("checked")),
	)

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{hasCheckedNote}}).
		List(&records))
	require.Len(t, records, 1, "only a1 has a tag carrying a checked note")
	require.Equal(t, "a1", records[0].ID)
}

// TestFilterExistsFailsClosedUnderNegation pins the one place in the renderer
// where fail-closed could invert. A predicate that cannot be applied becomes
// "match nothing"; placing that inside NOT EXISTS would turn it into "match
// everything", which is the only way this package could widen a result set.
// The whole condition has to collapse instead of the inner one.
func TestFilterExistsFailsClosedUnderNegation(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	list := func(t *testing.T, f types.Filter) int {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{f}}).
			List(&records))
		return len(records)
	}

	// An empty group cannot be rendered, so the subquery's filter fails closed.
	broken := types.FilterOr()

	t.Run("ExistsMatchesNothing", func(t *testing.T) {
		require.Equal(t, 0, list(t, types.FilterExists[*TestRecordTag](
			TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), broken,
		)))
	})

	t.Run("NotExistsAlsoMatchesNothing", func(t *testing.T) {
		require.Equal(t, 0, list(t, types.FilterNotExists[*TestRecordTag](
			TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), broken,
		)),
			"negating a fail-closed subquery must not return the whole table")
	})
}

// TestFilterExistsValidatesInnerColumns pins the subquery's filters to the
// related model's own columns. A name only the outer table has would otherwise
// resolve against the enclosing query and silently turn the condition into a
// correlated reference over different rows.
func TestFilterExistsValidatesInnerColumns(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// "status" exists on the record table but not on the tag table.
	outerOnly := types.FilterExists[*TestRecordTag](
		TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestAggregateRecordCols.Status.Eq("done"),
	)

	// The reference carries the record table, which the subquery over the
	// tags does not read: a wrong-model condition only service code could
	// have written, so the chain fails rather than correlating outward or
	// answering with an empty page.
	records := make([]*TestAggregateRecord, 0)
	err := database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{outerOnly}}).
		List(&records)
	require.ErrorIs(t, err, database.ErrColumnTable)
	require.ErrorContains(t, err, "the subquery over")

	// The select path reports the reason rather than answering with zero.
	got := struct{ Total int64 }{}
	require.ErrorIs(t, database.Select[*TestAggregateRecord, struct{ Total int64 }](ctx, TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(outerOnly).
		ScanOne(&got), database.ErrUnusableFilter)
}

// TestFilterExistsQualifiesInnerColumns pins the shape of the generated SQL
// rather than its result. SQL resolves an unqualified name against the
// innermost scope first, so a subquery filter reaches the right column either
// way and no query result can tell the two spellings apart. The qualification
// is still worth pinning: it is what keeps the emitted condition unambiguous
// on its face, and what would keep it correct if a subquery ever gained a join.
func TestFilterExistsQualifiesInnerColumns(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	// Both tables carry an "id"; the filter names the tag's own.
	byTagID := types.FilterExists[*TestRecordTag](
		TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.ID.Eq("t1"),
	)

	statements := make([]types.SQLStatement, 0)
	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](context.Background()).
		WithDryRun(&statements).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{byTagID}}).
		List(&records))

	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query, quoteIdent("test_record_tags")+"."+quoteIdent("id")+" =",
		"the inner filter must name the subquery's own table")
}

// TestFilterExistsSelfJoin covers a related model that reads the same table as
// the query around it. Without a distinct name for the subquery both sides of
// the correlation resolve to the inner table and the condition degenerates
// into comparing a row with itself.
func TestFilterExistsSelfJoin(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	ctx := context.Background()
	catCols := struct {
		ID       types.Column[string]
		ParentID types.Column[string]
	}{
		ID:       types.NewColumn[*TestCategory, string]("id"),
		ParentID: types.NewColumn[*TestCategory, string]("parent_id"),
	}
	require.NoError(t, database.Database[*TestCategory](ctx).Create(categoryRoot, categoryParent))

	// Categories that are somebody's parent. root parents itself and parent,
	// parent has no children, so only root matches.
	hasChild := types.FilterExists[*TestCategory](catCols.ParentID.EqCol(catCols.ID))
	cats := make([]*TestCategory, 0)
	require.NoError(t, database.Database[*TestCategory](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{hasChild}}).
		List(&cats))
	require.Len(t, cats, 1)
	require.Equal(t, categoryRootID, cats[0].ID)
}

// TestFilterExistsMultipleCorrelations covers a related model reached through
// a composite key: every EqCol must hold at once. A tag whose denormalized
// category disagrees with its record is reachable by record_id alone but not
// by the (record_id, category) pair. A correlation is a predicate like any
// other, so it also composes with FilterOr and has a string-column spelling;
// a subquery without one, or a correlation with nothing enclosing it, fails
// closed.
func TestFilterExistsMultipleCorrelations(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// a2 gets an "audit" tag whose category went stale (a2 is alpha), a3 gets
	// a consistent one.
	require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
		&TestRecordTag{ID: "t5", RecordID: "a2", Label: "audit", Category: "beta"},
		&TestRecordTag{ID: "t6", RecordID: "a3", Label: "audit", Category: "alpha"},
	))
	byRecord := TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID)
	byCategory := TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category)
	audit := TestRecordTagCols.Label.Eq("audit")
	audited := types.FilterExists[*TestRecordTag](byRecord, byCategory, audit)
	ids := func(filter types.Filter) []string {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{filter}}).
			WithOrder(types.Asc("id")).
			List(&records))
		got := make([]string, 0, len(records))
		for _, r := range records {
			got = append(got, r.ID)
		}
		return got
	}

	t.Run("RequiresEveryPair", func(t *testing.T) {
		require.Equal(t, []string{"a3"}, ids(audited), "a2's tag fails the category pair")
		require.Equal(t, []string{"a2", "a3"}, ids(types.FilterExists[*TestRecordTag](byRecord, audit)),
			"a single pair still reaches the stale tag")
	})

	t.Run("NegatesAsAWhole", func(t *testing.T) {
		unaudited := types.FilterNotExists[*TestRecordTag](byRecord, byCategory, audit)
		require.Equal(t, []string{"a1", "a2", "a4", "a5", "a6"}, ids(unaudited))
	})

	t.Run("MatchesAnyPairInsideOr", func(t *testing.T) {
		// t5 reaches a2 by record and every beta record by category; t6
		// reaches a3 by record and every alpha record by category.
		anyPair := types.FilterExists[*TestRecordTag](types.FilterOr(byRecord, byCategory), audit)
		require.Equal(t, []string{"a1", "a2", "a3", "a4", "a5"}, ids(anyPair))
	})

	t.Run("RendersPairsInOrder", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithDryRun(&statements).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{audited}}).
			List(&records))
		require.Len(t, statements, 1)
		sql := statements[0].Query
		tags, parents := quoteIdent("test_record_tags"), quoteIdent("test_aggregate_records")
		first := tags + "." + quoteIdent("record_id") + " = " + parents + "." + quoteIdent("id")
		second := tags + "." + quoteIdent("category") + " = " + parents + "." + quoteIdent("category")
		require.Contains(t, sql, first, "each side is qualified with its own table")
		require.Contains(t, sql, second)
		require.Less(t, strings.Index(sql, first), strings.Index(sql, second), "pairs render in the order given")
	})

	t.Run("FailsClosedWithoutCorrelation", func(t *testing.T) {
		require.Empty(t, ids(types.FilterExists[*TestRecordTag](audit)))
		// Negation would widen "match nothing" into "match everything", so
		// the whole condition still collapses to no rows.
		require.Empty(t, ids(types.FilterNotExists[*TestRecordTag](audit)))
	})

	t.Run("StringTierMatchesTypedTier", func(t *testing.T) {
		// FilterEqCol is what the column method delegates to, so naming the
		// columns as strings selects the same rows.
		byNames := types.FilterExists[*TestRecordTag](
			types.FilterEqCol("record_id", "id"), types.FilterEqCol("category", "category"), audit,
		)
		require.Equal(t, []string{"a3"}, ids(byNames))
	})

	t.Run("FailsClosedOnUnknownChildColumn", func(t *testing.T) {
		unknown := types.FilterExists[*TestRecordTag](
			byRecord, types.NewColumn[*TestRecordTag, string]("missing").EqCol(TestAggregateRecordCols.Category), audit,
		)
		require.Empty(t, ids(unknown))
	})

	t.Run("ParentOfAnotherTableIsRefused", func(t *testing.T) {
		// The users have an id too, so the name alone would tie the tags to
		// the records as valid SQL; the table the reference carries refuses
		// it, on the row path and on the select path alike.
		userID := types.NewColumn[*TestUser, string]("id")
		records := make([]*TestAggregateRecord, 0)
		require.ErrorIs(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
				types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(userID), audit),
			}}).
			List(&records), database.ErrColumnTable)
		type total struct{ Total int64 }
		err := database.Select[*TestAggregateRecord, total](ctx, TestAggregateRecordCols.Amount.Sum().As("total")).
			Where(types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(userID), audit)).
			ScanOne(&total{})
		require.ErrorIs(t, err, database.ErrColumnTable)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
	})

	t.Run("FailsClosedOutsideSubquery", func(t *testing.T) {
		// A plain-name EqCol at the top level has no enclosing query to tie
		// to and fails closed; the typed one names the tag table, which the
		// chain does not read, and fails the chain like any other reference
		// of another model.
		require.Empty(t, ids(types.FilterEqCol("record_id", "id")))
		records := make([]*TestAggregateRecord, 0)
		require.ErrorIs(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{byRecord}}).
			List(&records), database.ErrColumnTable)
	})

	t.Run("FailsClosedOnUnknownParentColumn", func(t *testing.T) {
		// The plain-name spelling cannot be checked at compile time, so the
		// renderer checks the outer side against the enclosing model instead
		// of letting the database answer with an unknown-column error.
		unknown := types.FilterExists[*TestRecordTag](
			byRecord, types.FilterEqCol("category", "missing"), audit,
		)
		require.Empty(t, ids(unknown))
	})
}
