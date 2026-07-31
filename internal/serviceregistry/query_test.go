package serviceregistry_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type querySample struct {
	Name   string `query:"name"`
	Age    int    `json:"age"`
	Active bool   `json:"active"`

	modelregistry.Query
	modelregistry.Base
}

type queryPlainSample struct {
	Name string `query:"name"`

	modelregistry.Base
}

// The fixtures must satisfy types.Model for a Base to accept them, which is
// what makes them usable as the service model below.
var (
	_ types.Model = (*querySample)(nil)
	_ types.Model = (*queryPlainSample)(nil)
)

// querySampleService stands for an application service that implements a list
// action itself and therefore parses the request through its own base.
type querySampleService struct {
	serviceregistry.Base[*querySample, *querySample, *querySample]
}

type queryPlainService struct {
	serviceregistry.Base[*queryPlainSample, *queryPlainSample, *queryPlainSample]
}

func TestBaseQueryDecode(t *testing.T) {
	var svc querySampleService

	t.Run("FillsModelFieldsAndFrameworkFields", func(t *testing.T) {
		m, err := svc.QueryDecode(newQueryContext(t, "/samples?name=alice&age=10&_page=2&_size=50"))
		require.NoError(t, err)
		require.Equal(t, "alice", m.Name)
		require.Equal(t, 10, m.Age)
		require.Equal(t, 2, m.Page)
		require.Equal(t, 50, m.Size)
	})

	t.Run("KeepsFilterKeysAwayFromTheDecoder", func(t *testing.T) {
		m, err := svc.QueryDecode(newQueryContext(t, "/samples?name=alice&age[gt]=20&created_at=2026-07-01"))
		require.NoError(t, err)
		require.Equal(t, "alice", m.Name)
	})

	t.Run("RejectsUnknownKeys", func(t *testing.T) {
		_, err := svc.QueryDecode(newQueryContext(t, "/samples?bogus=1"))
		require.Error(t, err, "an unknown key is a typo and must not be ignored")
	})

	t.Run("ReturnsAFreshModelEachCall", func(t *testing.T) {
		first, err := svc.QueryDecode(newQueryContext(t, "/samples?name=alice"))
		require.NoError(t, err)
		second, err := svc.QueryDecode(newQueryContext(t, "/samples?name=bob"))
		require.NoError(t, err)
		require.NotSame(t, first, second)
		require.Equal(t, "alice", first.Name, "a later call must not overwrite an earlier model")
	})
}

func TestBaseQueryFilters(t *testing.T) {
	var svc querySampleService

	t.Run("ParsesAgainstTheServiceModel", func(t *testing.T) {
		filters, err := svc.QueryFilters(newQueryContext(t, "/samples?age[gt]=20&name=alice"))
		require.NoError(t, err)
		require.Equal(t, []types.Filter{{Column: "age", Op: types.FilterOpGt, Value: "20"}}, filters)
	})

	t.Run("RejectsUnknownField", func(t *testing.T) {
		_, err := svc.QueryFilters(newQueryContext(t, "/samples?bogus[gt]=20"))
		require.Error(t, err)
	})

	// The model the filters are parsed against comes from the base's own type
	// parameter, so a model that never opted in to filtering rejects them.
	t.Run("ModelWithoutQueryRejectsFilters", func(t *testing.T) {
		var plain queryPlainService
		_, err := plain.QueryFilters(newQueryContext(t, "/samples?name[like]=a"))
		require.Error(t, err)
	})
}

func TestBaseQueryPresentFields(t *testing.T) {
	var svc querySampleService
	present := svc.QueryPresentFields(newQueryContext(t, "/samples?active=false&age[gt]=20&_page=2"))
	require.Equal(t, map[string]struct{}{"active": {}}, present,
		"only explicitly provided model filter keys are marked present")
}

func TestBaseQueryPagination(t *testing.T) {
	var svc querySampleService

	t.Run("ReadsRequestValues", func(t *testing.T) {
		page, size := svc.QueryPagination(newQueryContext(t, "/samples?_page=2&_size=50"))
		require.Equal(t, 2, page)
		require.Equal(t, 50, size)
	})

	t.Run("ClampsOversizedPageSize", func(t *testing.T) {
		_, size := svc.QueryPagination(newQueryContext(t, "/samples?_size=100000"))
		require.Equal(t, 100, size)
	})

	t.Run("ModelWithoutPaginationKeepsTheSafetyLimit", func(t *testing.T) {
		var plain queryPlainService
		page, size := plain.QueryPagination(newQueryContext(t, "/samples?_page=2&_size=50"))
		require.Equal(t, 0, page)
		require.Equal(t, 1000, size)
	})
}

func TestBaseQueryOrders(t *testing.T) {
	var svc querySampleService

	t.Run("ReadsRequestValues", func(t *testing.T) {
		orders, err := svc.QueryOrders(newQueryContext(t, "/samples?_sort_by=created_at%20desc"))
		require.NoError(t, err)
		require.Equal(t, []types.Order{{Column: "created_at", Direction: types.OrderDesc}}, orders)
	})

	t.Run("UnknownColumnIsAClientError", func(t *testing.T) {
		_, err := svc.QueryOrders(newQueryContext(t, "/samples?_sort_by=no_such_column"))
		require.Error(t, err)
	})

	t.Run("ModelWithoutQueryYieldsNoOrder", func(t *testing.T) {
		var plain queryPlainService
		orders, err := plain.QueryOrders(newQueryContext(t, "/samples?_sort_by=created_at%20desc"))
		require.NoError(t, err)
		require.Empty(t, orders)
	})
}

func TestBaseQueryCursor(t *testing.T) {
	var svc querySampleService

	t.Run("ReadsRequestValues", func(t *testing.T) {
		cursor, err := svc.QueryCursor(newQueryContext(t, "/samples?_cursor_value=2026-07-01T08:30:15&_cursor_next=true&_cursor_field=created_at"))
		require.NoError(t, err)
		require.Equal(t, types.CursorForward(types.Asc("created_at"), "2026-07-01T08:30:15"), cursor)
	})

	t.Run("UnknownColumnIsAClientError", func(t *testing.T) {
		_, err := svc.QueryCursor(newQueryContext(t, "/samples?_cursor_value=abc&_cursor_field=no_such_column"))
		require.Error(t, err)
	})

	t.Run("MistypedValueIsAClientError", func(t *testing.T) {
		_, err := svc.QueryCursor(newQueryContext(t, "/samples?_cursor_value=abc&_cursor_field=age"))
		require.Error(t, err)
	})

	t.Run("ModelWithoutCursorYieldsZeroCursor", func(t *testing.T) {
		var plain queryPlainService
		cursor, err := plain.QueryCursor(newQueryContext(t, "/samples?_cursor_value=abc&_cursor_next=true"))
		require.NoError(t, err)
		require.False(t, cursor.Enabled())
	})
}

// newQueryContext builds a service context carrying a GET request with the
// given target URL, for exercising the query parsing methods.
func newQueryContext(t *testing.T, target string) *types.ServiceContext {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return types.NewServiceContext(c, nil, "list")
}
