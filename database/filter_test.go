package database_test

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// Tests for the predicate engine in filter.go: the operator filters, their
// grouping and nesting, and the fail-closed rules. WithQuery's own behavior --
// how a model value becomes conditions -- is covered in with_query_test.go.

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
	listErr := func(filters ...types.Filter) error {
		users := make([]*TestUser, 0)
		return database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: filters}).
			List(&users)
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

	// An empty group is a caller bug, and groups are service code: the
	// chain fails on it rather than answering it with the logical identity
	// (TRUE for AND), which would widen the result set, or with an empty
	// page, which would pass the bug off as "no data". A child of a URL
	// operator that cannot be applied narrows the group instead, like any
	// filter a client could have sent.
	t.Run("MalformedGroupsFailTheChain", func(t *testing.T) {
		require.ErrorIs(t, listErr(types.FilterOr()), database.ErrUnusableFilter, "empty OR group")
		require.ErrorIs(t, listErr(types.FilterAnd()), database.ErrUnusableFilter, "empty AND group")
		require.ErrorIs(t, listErr(types.NewFilter("", "", types.FilterOpOr, "oops")), database.ErrUnusableFilter,
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
				types.NewFilter("", "age", types.FilterOpGt, "18"),
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
				types.NewFilter("", "age", types.FilterOpGte, "18"),
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
				types.NewFilter("", "age", types.FilterOpGt, "18"),
			},
		}).
		List(&users))
	require.Empty(t, users, "conditions combine with exact filters using AND logic")

	// Test eq: age = 19 should return u2.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "age", types.FilterOpEq, "19"),
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
				types.NewFilter("", "age", types.FilterOpNe, "19"),
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
				types.NewFilter("", "age", types.FilterOpLt, "19"),
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "age", types.FilterOpLte, "19"),
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
				types.NewFilter("", "email", types.FilterOpLike, "@example"),
			},
		}).
		List(&users))
	require.Len(t, users, 3, "like should match substrings")

	// Test notlike: name not like "1" should return u2 and u3.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "name", types.FilterOpNotLike, "1"),
			},
		}).
		List(&users))
	require.Len(t, users, 2, "notlike should exclude substring matches")

	// Test in and notin: comma-separated values split into a set.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "age", types.FilterOpIn, []string{"18", "20"}),
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
				types.NewFilter("", "name", types.FilterOpNotIn, []string{"user1", "user2"}),
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
				types.NewFilter("", "name", types.FilterOpStartsWith, "user"),
			},
		}).
		List(&users))
	require.Len(t, users, 3)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "name", types.FilterOpStartsWith, "ser"),
			},
		}).
		List(&users))
	require.Empty(t, users, "startswith must anchor at the beginning of the value")

	// Test endswith is anchored at the end.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "email", types.FilterOpEndsWith, "1@example.com"),
			},
		}).
		List(&users))
	require.Len(t, users, 1)
	require.Equal(t, u1.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "email", types.FilterOpEndsWith, "user"),
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
				types.NewFilter("", "is_active", types.FilterOpIsNull, true),
			},
		}).
		List(&users))
	require.Len(t, users, 3, "isnull=1 must select records whose column IS NULL")

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "is_active", types.FilterOpIsNull, false),
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
				types.NewFilter("", "name", types.FilterOpLike, "user_"),
			},
		}).
		List(&users))
	require.Len(t, users, 1, "LIKE metacharacters in the value must be escaped")
	require.Equal(t, underscoreUser.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "name", types.FilterOpStartsWith, "user_"),
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
				types.NewFilter("", "name", types.FilterOpRegex, "^user[12]$"),
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
				types.NewFilter("", "name", types.FilterOpNotRegex, "^user[0-9]$"),
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
				types.NewFilter("", "name", types.FilterOpRegex, "(unclosed"),
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
				types.NewFilter("", "addr", types.FilterOpJSONContains, "alpha"),
			},
		}).
		List(&users))
	require.Len(t, users, 1, "jsoncontains must match JSON array membership")
	require.Equal(t, addrUser.ID, users[0].ID)

	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "addr", types.FilterOpJSONContains, "gamma"),
			},
		}).
		List(&users))
	require.Empty(t, users, "jsoncontains must not match absent members")

	// jsoncontains is an operator only service code writes, so on a column
	// that stores no JSON document the chain fails, instead of answering the
	// empty page that would pass the mistake off as "no data" or reaching
	// the database as a SQL error.
	users = make([]*TestUser, 0)
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpJSONContains, "user6")},
		}).
		List(&users), database.ErrUnusableFilter, "jsoncontains on a column storing no JSON document must fail the chain")

	// An unknown operator is no client's either: the URL decoder admits only
	// the operators it knows, so the chain fails on it too.
	users = make([]*TestUser, 0)
	require.ErrorIs(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "age", types.FilterOp("bogus"), "1"),
			},
		}).
		List(&users), database.ErrUnusableFilter, "unknown operator must fail the chain")

	// A filter of an operator a client can spell in a URL narrows the query
	// to an empty result when it cannot be applied, whoever wrote it: an
	// empty column adds "1 = 0" instead of failing the request.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "", types.FilterOpEq, "1"),
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
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIn, []string{u1.Name, u3.Name})},
		}).
		List(&users))
	require.Len(t, users, 2, "slice values must bind IN directly")

	// A slice of a named string type binds the same way as []string, so
	// enum-typed values need no per-member conversion.
	type sampleStatus string
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIn, []sampleStatus{sampleStatus(u1.Name), sampleStatus(u3.Name)})},
		}).
		List(&users))
	require.Len(t, users, 2, "named string type slices must bind like []string")

	// A nil slice, which a variadic Column.In() call with no arguments
	// produces, must behave like an empty one.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIn, []string(nil))},
		}).
		List(&users))
	require.Empty(t, users, "a nil slice must match nothing")

	// An empty slice matches nothing instead of widening the result set.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIn, []string{})},
		}).
		List(&users))
	require.Empty(t, users, "empty slice must match nothing")

	// In does not split comma strings: a string value fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIn, u1.Name+","+u2.Name)},
		}).
		List(&users))
	require.Empty(t, users, "string value on In must fail closed")

	// A scalar comparison rejects slice values and fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "age", types.FilterOpGt, []int{18})},
		}).
		List(&users))
	require.Empty(t, users, "slice value on a scalar operator must fail closed")

	// A nil value fails closed on every operator family.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "age", types.FilterOpEq, nil)},
		}).
		List(&users))
	require.Empty(t, users, "nil value must fail closed")

	// IsNull carries a bool: false means IS NOT NULL and matches every row
	// of a NOT NULL column, while a "1" string fails closed.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIsNull, false)},
		}).
		List(&users))
	require.Len(t, users, 3, "IsNull false must mean IS NOT NULL")
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{types.NewFilter("", "name", types.FilterOpIsNull, "1")},
		}).
		List(&users))
	require.Empty(t, users, "string value on IsNull must fail closed")

	// Typed Go values bind directly on comparison operators.
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).
		WithQuery(nil, types.QueryOptions{
			Filters: []types.Filter{
				types.NewFilter("", "age", types.FilterOpGt, 18),
				types.NewFilter("", "created_at", types.FilterOpGte, time.Now().Add(-time.Hour)),
			},
		}).
		List(&users))
	require.Len(t, users, 2, "typed scalar values must bind directly")
}

// TestDatabaseScalarFiltersOnBytesAndValuers pins that a comparison binds a
// byte slice, and a value that binds itself through driver.Valuer such as a
// token held in an array, as the one value it is, the way gorm's own
// parameter binding does, where any other slice or array value fails closed;
// and that a JSON array column keeps failing closed on a comparison.
func TestDatabaseScalarFiltersOnBytesAndValuers(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_scored_records").Error })
	require.NoError(t, database.Database[*TestScoredRecord](ctx).Create(
		&TestScoredRecord{Name: "first", Digest: []byte{0x00, 0xff, 0x10}, Token: sampleToken{1}, Scores: datatypes.NewJSONSlice([]int{1, 2})},
		&TestScoredRecord{Name: "second", Digest: []byte{0x01}, Token: sampleToken{2}, Scores: datatypes.NewJSONSlice([]int{3})},
	))
	names := func(filter types.Filter) []string {
		rows := make([]*TestScoredRecord, 0)
		require.NoError(t, database.Database[*TestScoredRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{filter}}).
			WithOrder(TestScoredRecordCols.Name.Asc()).
			List(&rows))
		names := make([]string, 0, len(rows))
		for _, row := range rows {
			names = append(names, row.Name)
		}
		return names
	}

	require.Equal(t, []string{"first"}, names(TestScoredRecordCols.Digest.Eq([]byte{0x00, 0xff, 0x10})), "a byte slice is one value")
	require.Equal(t, []string{"second"}, names(TestScoredRecordCols.Digest.Ne([]byte{0x00, 0xff, 0x10})))
	require.Equal(t, []string{"first"}, names(TestScoredRecordCols.Token.Eq(sampleToken{1})), "a Valuer held in an array is one value")
	require.Equal(t, []string{"second"}, names(TestScoredRecordCols.Token.Ne(sampleToken{1})))
	require.Equal(t, []string{"second"}, names(TestScoredRecordCols.Token.Gt(sampleToken{1})))
	require.Equal(t, []string{"first", "second"}, names(TestScoredRecordCols.Token.In(sampleToken{1}, sampleToken{2})))
	require.Empty(t, names(TestScoredRecordCols.Scores.Eq(datatypes.NewJSONSlice([]int{1, 2}))), "a JSON array column keeps failing closed on a comparison")
}

// TestDatabasePatternFiltersOnNumericColumns pins that the like family and
// the regex operators match a numeric column by the number's text form on
// every dialect: the same request answers the same rows whether the dialect
// reads a number in a string context by itself or needs the column cast.
func TestDatabasePatternFiltersOnNumericColumns(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	ages := func(filter types.Filter) []int {
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{filter}}).
			WithOrder(types.NewColumn[*TestUser, int]("age").Asc()).
			List(&users))
		ages := make([]int, 0, len(users))
		for _, u := range users {
			ages = append(ages, u.Age)
		}
		return ages
	}

	require.Equal(t, []int{18, 19}, ages(types.NewFilter("", "age", types.FilterOpLike, "1")))
	require.Equal(t, []int{20}, ages(types.NewFilter("", "age", types.FilterOpNotLike, "1")))
	require.Equal(t, []int{20}, ages(types.NewFilter("", "age", types.FilterOpStartsWith, "2")))
	require.Equal(t, []int{19}, ages(types.NewFilter("", "age", types.FilterOpEndsWith, "9")))
	require.Equal(t, []int{18, 19}, ages(types.NewFilter("", "age", types.FilterOpRegex, "^1")))
	require.Equal(t, []int{20}, ages(types.NewFilter("", "age", types.FilterOpNotRegex, "^1")))

	// Inside a subquery the column belongs to the subquery's own table, read
	// under its alias, and still matches by the number's text form.
	inSubquery := func(filter types.Filter) types.Filter {
		return types.FilterExists[*TestUser](types.FilterEqCol("id", "id"), filter)
	}
	require.Equal(t, []int{18, 19}, ages(inSubquery(types.NewFilter("", "age", types.FilterOpLike, "1"))))
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

// TestDatabaseFiltersOnDateColumns pins the runtime behavior of comparisons
// on a date column across dialects, declared a date by its type or by its
// type tag alike: the URL parser binds an instant in types.FilterTimeLayout,
// service code binds a time.Time through the time reference gg gen writes for
// the column, and both compare by the UTC day the value falls on, selecting
// the rows of that day however the dialect stores the date. The same
// reference buckets the column, so a report groups by it.
func TestDatabaseFiltersOnDateColumns(t *testing.T) {
	defer cleanupDatedData()
	day := func(d int) time.Time { return time.Date(2026, time.January, d, 0, 0, 0, 0, time.UTC) }
	due := func(d int) *time.Time { t := day(d); return &t }
	require.NoError(t, database.Database[*TestDatedRecord](context.Background()).Create(
		&TestDatedRecord{Label: "d1", Day: datatypes.Date(day(1)), Due: due(1)},
		&TestDatedRecord{Label: "d2", Day: datatypes.Date(day(2)), Due: due(2)},
		&TestDatedRecord{Label: "d3", Day: datatypes.Date(day(3)), Due: due(3)},
	))

	labels := func(t *testing.T, f types.Filter) []string {
		t.Helper()
		records := make([]*TestDatedRecord, 0)
		require.NoError(t, database.Database[*TestDatedRecord](context.Background()).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{f}}).
			WithOrder(types.Asc("label")).
			List(&records))
		labels := make([]string, 0, len(records))
		for _, r := range records {
			labels = append(labels, r.Label)
		}
		return labels
	}

	t.Run("CanonicalStringBound", func(t *testing.T) {
		require.Equal(t, []string{"d2"}, labels(t, types.FilterEq("day", "2026-01-02 00:00:00")))
		require.Equal(t, []string{"d2", "d3"}, labels(t, types.FilterGte("day", "2026-01-02 00:00:00")))
	})

	t.Run("TimeValueBound", func(t *testing.T) {
		require.Equal(t, []string{"d2"}, labels(t, TestDatedRecordCols.Day.Eq(day(2))))
		require.Equal(t, []string{"d1"}, labels(t, TestDatedRecordCols.Day.Lt(day(2))))
	})

	t.Run("InstantsCompareByTheirUTCDay", func(t *testing.T) {
		// The start of the 2nd at +08:00 is 16:00 on the 1st in UTC, so it
		// names the 1st; 15:59 on the 2nd in UTC names the 2nd.
		for _, column := range []string{"day", "due"} {
			require.Equal(t, []string{"d1", "d2", "d3"}, labels(t, types.FilterGte(column, "2026-01-01 16:00:00")), column)
			require.Equal(t, []string{"d2"}, labels(t, types.FilterEq(column, "2026-01-02 16:00:00")), column)
			require.Equal(t, []string{"d1", "d2"}, labels(t, types.FilterLte(column, time.Date(2026, time.January, 2, 15, 59, 0, 0, time.UTC))), column)
		}
		require.Equal(t, []string{"d2", "d3"}, labels(t, TestDatedRecordCols.Due.Gt(time.Date(2026, time.January, 1, 23, 0, 0, 0, time.UTC))))
		// The members of an in or a notin list name their days the same way.
		members := []time.Time{time.Date(2026, time.January, 1, 16, 0, 0, 0, time.UTC), time.Date(2026, time.January, 3, 9, 0, 0, 0, time.UTC)}
		require.Equal(t, []string{"d1", "d3"}, labels(t, TestDatedRecordCols.Due.In(members...)))
		require.Equal(t, []string{"d2"}, labels(t, TestDatedRecordCols.Day.NotIn(members...)))
	})

	t.Run("BucketsByDay", func(t *testing.T) {
		type row struct {
			Bucket  string
			Records int64
		}
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestDatedRecord, row](context.Background(),
			TestDatedRecordCols.Day.ByDay().As("bucket"),
			types.Count().As("records"),
		).
			OrderBy(TestDatedRecordCols.Day.ByDay().As("bucket").Asc()).
			Scan(&rows))
		require.Equal(t, []row{
			{Bucket: "2026-01-01", Records: 1},
			{Bucket: "2026-01-02", Records: 1},
			{Bucket: "2026-01-03", Records: 1},
		}, rows)
	})
}

// TestDatabaseFiltersFollowTheStoredType pins that the filters read a column
// by the type gorm stores, not by its Go shape, alike on every dialect: the
// time sql.NullTime holds compares as a time, the integer sql.NullInt64
// holds and a string its type tag declares a JSON column match a pattern by
// their text form, and a time of day compares in the spelling datatypes.Time
// stores, the bound the URL parser hands over.
func TestDatabaseFiltersFollowTheStoredType(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_typed_records").Error })
	settled := time.Date(2026, time.July, 1, 8, 0, 0, 0, time.UTC)
	require.NoError(t, database.Database[*TestTypedRecord](ctx).Create(
		&TestTypedRecord{
			Label: "first", Settled: sql.NullTime{Time: settled, Valid: true}, Tally: sql.NullInt64{Int64: 15, Valid: true},
			Payload: `{"code":"alpha"}`, Clock: datatypes.NewTime(9, 0, 0, 0),
		},
		&TestTypedRecord{
			Label: "second", Settled: sql.NullTime{Time: settled.Add(time.Hour), Valid: true}, Tally: sql.NullInt64{Int64: 27, Valid: true},
			Payload: `{"code":"beta"}`, Clock: datatypes.NewTime(17, 30, 0, 0),
		},
	))
	labels := func(t *testing.T, f types.Filter) []string {
		t.Helper()
		records := make([]*TestTypedRecord, 0)
		require.NoError(t, database.Database[*TestTypedRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{f}}).
			WithOrder(types.Asc("label")).
			List(&records))
		labels := make([]string, 0, len(records))
		for _, r := range records {
			labels = append(labels, r.Label)
		}
		return labels
	}

	t.Run("AWrappedTimeComparesAsATime", func(t *testing.T) {
		bound := settled.Format(types.FilterTimeLayout)
		require.Equal(t, []string{"first"}, labels(t, types.FilterEq("settled", bound)))
		require.Equal(t, []string{"second"}, labels(t, types.FilterGt("settled", bound)))
	})

	t.Run("AWrappedNumberMatchesAPatternByItsText", func(t *testing.T) {
		require.Equal(t, []string{"first"}, labels(t, types.FilterLike("tally", "5")))
	})

	t.Run("ATypeTaggedJSONColumnMatchesAPatternByItsText", func(t *testing.T) {
		require.Equal(t, []string{"second"}, labels(t, types.FilterLike("payload", "beta")))
	})

	t.Run("ATimeOfDayComparesInItsStoredSpelling", func(t *testing.T) {
		require.Equal(t, []string{"first"}, labels(t, types.FilterEq("clock", "09:00:00")))
		require.Equal(t, []string{"second"}, labels(t, types.FilterGte("clock", "12:00:00")))
	})
}

// TestURLFiltersReachEmbeddedColumnsByPath pins, on every dialect, that a URL
// filter names a column of a struct embedded by name by the path the field
// takes in JSON: first.label filters the first part's label and label the
// model's own, however the three share a field name. A bare key, the exact
// match a model takes without embedding Query, reaches the same columns by
// the same names.
func TestURLFiltersReachEmbeddedColumnsByPath(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, database.DB().AutoMigrate(&TestPairedRecord{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&TestPairedRecord{}))
	})
	require.NoError(t, database.Database[*TestPairedRecord](ctx).Create(
		&TestPairedRecord{Label: "l1", First: TestPart{Label: "f1"}, Second: TestPart{Label: "s1"}},
		&TestPairedRecord{Label: "f1", First: TestPart{Label: "s1"}, Second: TestPart{Label: "l1"}},
	))
	labels := func(t *testing.T, key, value string) []string {
		t.Helper()
		filters, err := urlquery.Filters(url.Values{key: {value}}, &TestPairedRecord{})
		require.NoError(t, err)
		records := make([]*TestPairedRecord, 0)
		require.NoError(t, database.Database[*TestPairedRecord](ctx).
			WithQuery(nil, types.QueryOptions{Filters: filters}).
			WithOrder(types.Asc("label")).
			List(&records))
		labels := make([]string, 0, len(records))
		for _, r := range records {
			labels = append(labels, r.Label)
		}
		return labels
	}

	require.Equal(t, []string{"l1"}, labels(t, "label[eq]", "l1"), "label is the model's own column")
	require.Equal(t, []string{"l1"}, labels(t, "first.label[eq]", "f1"))
	require.Equal(t, []string{"f1"}, labels(t, "second.label[eq]", "l1"))

	bare := func(t *testing.T, key, value string) []string {
		t.Helper()
		q := url.Values{key: {value}}
		query := &TestPairedRecord{}
		require.NoError(t, urlquery.Decode(q, query))
		records := make([]*TestPairedRecord, 0)
		require.NoError(t, database.Database[*TestPairedRecord](ctx).
			WithQuery(query, types.QueryOptions{PresentFields: urlquery.PresentFields(q)}).
			WithOrder(types.Asc("label")).
			List(&records))
		labels := make([]string, 0, len(records))
		for _, r := range records {
			labels = append(labels, r.Label)
		}
		return labels
	}
	require.Equal(t, []string{"f1"}, bare(t, "label", "f1"))
	require.Equal(t, []string{"l1"}, bare(t, "first.label", "f1"))
	require.Equal(t, []string{"f1"}, bare(t, "second.label", "l1"))
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
	// A subquery over the chain's own table is a subquery still: the
	// message names it, not the model around it.
	self := types.FilterExists[*TestAggregateRecord](
		types.NewColumn[*TestAggregateRecord, string]("category").EqCol(TestAggregateRecordCols.Category),
		TestRecordTagCols.Label.Eq("vip"),
	)
	err := database.Database[*TestAggregateRecord](context.Background()).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{self}}).
		List(&records)
	require.ErrorIs(t, err, database.ErrColumnTable)
	require.ErrorContains(t, err, `the subquery over "test_aggregate_records"`)
}

// The semi-join tests below cover FilterExists and FilterNotExists over the
// aggregate fixture: the records and the tags that point at them, described at
// aggregateSeed and tagSeed in fixtures_test.go.

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

// TestFilterExistsFailsTheChainOnAnUnusableSubquery pins what an EXISTS
// whose subquery cannot be rendered does to the chain: it fails it, negated
// or not. A subquery is service code, so answering "match nothing" would
// pass the mistake off as "no data"; negating that answer would even turn it
// into "match everything", the one way this package could widen a result
// set.
func TestFilterExistsFailsTheChainOnAnUnusableSubquery(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	list := func(t *testing.T, f types.Filter) error {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		return database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{f}}).
			List(&records)
	}

	// An empty group cannot be rendered, so the subquery's filter is unusable.
	broken := types.FilterOr()

	t.Run("Exists", func(t *testing.T) {
		require.ErrorIs(t, list(t, types.FilterExists[*TestRecordTag](
			TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), broken,
		)), database.ErrUnusableFilter)
	})

	t.Run("NotExists", func(t *testing.T) {
		require.ErrorIs(t, list(t, types.FilterNotExists[*TestRecordTag](
			TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), broken,
		)), database.ErrUnusableFilter, "negating an unusable subquery must not return the whole table")
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
	listErr := func(filter types.Filter) error {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		return database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{filter}}).
			List(&records)
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

	t.Run("FailsWithoutCorrelation", func(t *testing.T) {
		// A subquery tied to nothing is service code gone wrong, so the chain
		// fails rather than answering an empty page; negated as well, where
		// "match nothing" would otherwise widen into "match everything".
		require.ErrorIs(t, listErr(types.FilterExists[*TestRecordTag](audit)), database.ErrUnusableFilter)
		require.ErrorIs(t, listErr(types.FilterNotExists[*TestRecordTag](audit)), database.ErrUnusableFilter)
	})

	t.Run("StringTierMatchesTypedTier", func(t *testing.T) {
		// FilterEqCol is what the column method delegates to, so naming the
		// columns as strings selects the same rows.
		byNames := types.FilterExists[*TestRecordTag](
			types.FilterEqCol("record_id", "id"), types.FilterEqCol("category", "category"), audit,
		)
		require.Equal(t, []string{"a3"}, ids(byNames))
	})

	t.Run("FailsOnUnknownChildColumn", func(t *testing.T) {
		unknown := types.FilterExists[*TestRecordTag](
			byRecord, types.NewColumn[*TestRecordTag, string]("missing").EqCol(TestAggregateRecordCols.Category), audit,
		)
		require.ErrorIs(t, listErr(unknown), database.ErrUnusableFilter)
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

		// Both sides on the subquery's own table tie it to nothing enclosing;
		// the message says which table was expected.
		err = database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
				types.FilterExists[*TestRecordTag](byRecord, TestRecordTagCols.Label.EqCol(TestRecordTagCols.Category)),
			}}).
			List(&records)
		require.ErrorIs(t, err, database.ErrColumnTable)
		require.ErrorContains(t, err, `is not the table enclosing the subquery over "test_record_tags"`)
	})

	t.Run("FailsOutsideSubquery", func(t *testing.T) {
		// A plain-name EqCol at the top level has no enclosing query to tie
		// to and fails the chain; the typed one names the tag table, which
		// the chain does not read, and fails it like any other reference of
		// another model.
		require.ErrorIs(t, listErr(types.FilterEqCol("record_id", "id")), database.ErrUnusableFilter)
		records := make([]*TestAggregateRecord, 0)
		require.ErrorIs(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{byRecord}}).
			List(&records), database.ErrColumnTable)
	})

	t.Run("FailsOnUnknownParentColumn", func(t *testing.T) {
		// The plain-name spelling cannot be checked at compile time, so the
		// renderer checks the outer side against the enclosing model instead
		// of letting the database answer with an unknown-column error.
		unknown := types.FilterExists[*TestRecordTag](
			byRecord, types.FilterEqCol("category", "missing"), audit,
		)
		require.ErrorIs(t, listErr(unknown), database.ErrUnusableFilter)
	})
}

// TestLikeFiltersCompareCase pins that the like family compares case on every
// dialect: a pattern in one case matches no row spelled in the other, so a
// project testing on sqlite sees the rows MySQL and postgres answer.
func TestLikeFiltersCompareCase(t *testing.T) {
	defer cleanupTestData()
	ctx := context.Background()
	rows := []*TestUser{
		{Name: "case-lower", Email: "lower@example.com"},
		{Name: "Case-Upper", Email: "upper@example.com"},
	}
	require.NoError(t, database.Database[*TestUser](ctx).Create(rows...))

	names := func(filter types.Filter) []string {
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{filter}}).
			List(&users))
		got := make([]string, 0, len(users))
		for _, u := range users {
			got = append(got, u.Name)
		}
		slices.Sort(got)
		return got
	}
	require.Equal(t, []string{"case-lower"}, names(types.NewFilter("", "name", types.FilterOpLike, "case")))
	require.Equal(t, []string{"Case-Upper"}, names(types.NewFilter("", "name", types.FilterOpNotLike, "case")))
	require.Equal(t, []string{"Case-Upper"}, names(types.NewFilter("", "name", types.FilterOpStartsWith, "Case")))
	require.Equal(t, []string{"case-lower"}, names(types.NewFilter("", "name", types.FilterOpEndsWith, "lower")))
}
