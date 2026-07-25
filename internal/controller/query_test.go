package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

type listQueryableTestModel struct {
	Name string `query:"name"`

	modelregistry.Query
	modelregistry.Base
}

type listUnsafeQueryableTestModel struct {
	Name string `query:"name"`

	modelregistry.Query
	modelregistry.UnsafeQuery
	modelregistry.Base
}

func TestDecodeListQueryGatesUnsafeQueryKeys(t *testing.T) {
	queryKeys := map[string][]string{
		"name":     {"alice"},
		"_sort_by": {"created_at desc"},
	}

	t.Run("QueryAcceptsRegularKeys", func(t *testing.T) {
		var m listQueryableTestModel
		require.NoError(t, decodeListQuery(&m, queryKeys))
	})

	t.Run("QueryRejectsUnsafeKeys", func(t *testing.T) {
		for _, key := range []string{"_or", "_index", "_select", "_no_total"} {
			var m listQueryableTestModel
			err := decodeListQuery(&m, map[string][]string{key: {"true"}})
			require.Error(t, err, "unsafe query key %q must be rejected without modelregistry.UnsafeQuery", key)
		}
	})

	t.Run("UnsafeQueryAcceptsUnsafeKeys", func(t *testing.T) {
		var m listUnsafeQueryableTestModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{
			"_or":       {"true"},
			"_index":    {"idx_test"},
			"_select":   {"name"},
			"_no_total": {"true"},
		}))
	})

	t.Run("UnsafeQueryAloneRejectsRegularKeys", func(t *testing.T) {
		type unsafeOnlyModel struct {
			modelregistry.UnsafeQuery
			modelregistry.Base
		}
		var m unsafeOnlyModel
		require.Error(t, decodeListQuery(&m, map[string][]string{"_sort_by": {"created_at desc"}}))
	})
}

type expandQueryTestModel struct {
	Children   []*expandQueryTestModel
	Parent     *expandQueryTestModel
	ChildItems []*expandQueryTestModel

	modelregistry.Base
}

func (*expandQueryTestModel) Expands() []string { return []string{"Children", "Parent", "ChildItems"} }

func TestParseExpandQuery(t *testing.T) {
	t.Run("DepthRepeatsSliceExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=3")
		require.Equal(t, []string{"Children.Children.Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NonSliceExpandIgnoresDepth", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Parent&_depth=3")
		require.Equal(t, []string{"Parent"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("AllSelectsEveryModelExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=all")
		require.Equal(t, []string{"Children", "Parent", "ChildItems"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("ExpandMatchesCaseInsensitively", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=children")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("ExpandMatchesSnakeCaseName", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=child_items")
		require.Equal(t, []string{"ChildItems"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("DepthAcceptsUpperBoundTen", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=10")
		require.Equal(t, []string{strings.Repeat("Children.", 9) + "Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("UnknownExpandDropped", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Bogus")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("OutOfRangeDepthFallsBackToOne", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=11")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NoExpandParameterReturnsNothing", func(t *testing.T) {
		c := newTestGetContext(t, "/items")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})
}

type filterKeyTestModel struct {
	Name string `query:"name"`
	Age  int    `json:"age"`

	modelregistry.Query
	modelregistry.Base
}

func TestDecodeListQueryPageSizeGating(t *testing.T) {
	type cursorOnlyModel struct {
		Name string `query:"name"`

		modelregistry.Cursor
		modelregistry.Base
	}
	type paginatableModel struct {
		Name string `query:"name"`

		modelregistry.Pagination
		modelregistry.Base
	}
	type plainModel struct {
		Name string `query:"name"`

		modelregistry.Base
	}

	t.Run("CursorModelAcceptsSizeButRejectsPage", func(t *testing.T) {
		var m cursorOnlyModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{"_size": {"50"}}),
			"cursor pagination needs a client-adjustable batch size")
		require.Error(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}}),
			"offset paging conflicts with cursor semantics")
	})

	t.Run("PaginatableModelAcceptsBoth", func(t *testing.T) {
		var m paginatableModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}, "_size": {"50"}}))
	})

	t.Run("PlainModelRejectsBoth", func(t *testing.T) {
		var m plainModel
		require.Error(t, decodeListQuery(&m, map[string][]string{"_size": {"50"}}))
		require.Error(t, decodeListQuery(&m, map[string][]string{"_page": {"2"}}))
	})
}

func TestDecodeListQueryIgnoresFilterKeys(t *testing.T) {
	var m filterKeyTestModel
	require.NoError(t, decodeListQuery(&m, map[string][]string{
		"name":       {"alice"},
		"age":        {"10"},
		"age[gt]":    {"20"},
		"created_at": {"2026-07-01"},
	}))
	require.Equal(t, "alice", m.Name)
	require.Equal(t, 10, m.Age,
		"the bare key keeps feeding the exact business filter while its operator key is left to urlquery.Filters")
}

// newTestGetContext builds a gin context carrying a GET request with the given
// target URL, for exercising query-parameter parsing helpers.
func newTestGetContext(t *testing.T, target string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}
