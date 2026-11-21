package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// strictQueryResult runs one request with the given raw query through the gate
// and reports the answered status and body.
func strictQueryResult(t *testing.T, method, rawQuery string) (int, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(strictQuery())
	handle := func(c *gin.Context) { c.Status(http.StatusOK) }
	engine.GET("/probe", handle)
	engine.POST("/probe", handle)

	target := "/probe"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))

	return recorder.Code, recorder.Body.String()
}

func TestStrictQueryAdmitsUnambiguousQueries(t *testing.T) {
	cases := []struct {
		name     string
		rawQuery string
	}{
		{name: "no query string", rawQuery: ""},
		{name: "single-valued keys", rawQuery: "box_id=a&_page=1&_size=10"},
		{name: "bracketed range keys are distinct keys", rawQuery: "created_at[gte]=2026-01-01T00:00:00Z&created_at[lte]=2026-02-01T00:00:00Z"},
		{name: "same value under different keys", rawQuery: "a=1&b=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, _ := strictQueryResult(t, http.MethodGet, c.rawQuery)
			require.Equal(t, http.StatusOK, status)
		})
	}
}

func TestStrictQueryRejectsRepeatedKeys(t *testing.T) {
	status, body := strictQueryResult(t, http.MethodGet, "box_id=a&box_id=b")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "duplicate query parameter: box_id")
}

// TestStrictQueryRejectsEncodedKeyVariants pins that repetition is judged on
// the decoded key: an escaped spelling of the same key must not slip past the
// gate, or a crafted URL could still drive two decoded readers apart.
func TestStrictQueryRejectsEncodedKeyVariants(t *testing.T) {
	status, body := strictQueryResult(t, http.MethodGet, "box%5Fid=a&box_id=b")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "duplicate query parameter: box_id")
}

// TestStrictQueryListsRepeatedKeysSorted pins a stable refusal message: map
// iteration order is random, so the listed keys must come out sorted.
func TestStrictQueryListsRepeatedKeysSorted(t *testing.T) {
	status, body := strictQueryResult(t, http.MethodGet, "b=1&b=2&a=1&a=2")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "duplicate query parameter: a, b")
}

func TestStrictQueryRejectsMalformedQueries(t *testing.T) {
	cases := []struct {
		name     string
		rawQuery string
	}{
		{name: "stray percent escape", rawQuery: "a=%zz"},
		{name: "semicolon separator", rawQuery: "a=1;b=2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := strictQueryResult(t, http.MethodGet, c.rawQuery)
			require.Equal(t, http.StatusBadRequest, status)
			require.Contains(t, body, "malformed query string")
		})
	}
}

// TestStrictQueryRefusesAPageOrSizeThatIsNoCount pins that _page and _size
// carry a count, zero or more, wherever a request gives them: one that is no
// integer, or a negative one, is refused at the gate, before a list flow or
// a service reads it as the default, naming the parameter and echoing the
// value alone: one that is no integer in the words a list of the
// framework's own answers a value its model cannot decode, a negative one
// as one. Zero and an empty value are admitted, each read as the parameter
// left out, and so is a size past the cap, which the list brings down to
// it.
func TestStrictQueryRefusesAPageOrSizeThatIsNoCount(t *testing.T) {
	for _, tt := range []struct{ name, method, rawQuery, wantMsg string }{
		{name: "page that is no number", method: http.MethodGet, rawQuery: "_page=abc", wantMsg: `invalid query parameter "_page": expect a numeric value, got "abc"`},
		{name: "negative page", method: http.MethodGet, rawQuery: "_page=-1", wantMsg: `invalid query parameter "_page": expect a non-negative value, got "-1"`},
		{name: "size that is no integer", method: http.MethodGet, rawQuery: "_size=1.5", wantMsg: `invalid query parameter "_size": expect a numeric value, got "1.5"`},
		{name: "negative size", method: http.MethodGet, rawQuery: "_size=-5", wantMsg: `invalid query parameter "_size": expect a non-negative value, got "-5"`},
		{name: "page past what an int holds", method: http.MethodGet, rawQuery: "_page=18446744073709551615", wantMsg: `invalid query parameter "_page": expect a numeric value, got "18446744073709551615"`},
		{name: "page of a request that lists nothing", method: http.MethodPost, rawQuery: "_page=abc", wantMsg: `invalid query parameter "_page": expect a numeric value, got "abc"`},
		{name: "zero page and size", method: http.MethodGet, rawQuery: "_page=0&_size=0"},
		{name: "empty page and size", method: http.MethodGet, rawQuery: "_page=&_size="},
		{name: "size past the cap", method: http.MethodGet, rawQuery: "_page=2&_size=500"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status, body := strictQueryResult(t, tt.method, tt.rawQuery)
			if tt.wantMsg == "" {
				require.Equal(t, http.StatusOK, status)
				return
			}
			require.Equal(t, http.StatusBadRequest, status)
			var envelope struct {
				Msg string `json:"msg"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &envelope))
			require.Equal(t, tt.wantMsg, envelope.Msg)
		})
	}
}

// TestStrictQueryCoversEveryMethod pins that the gate judges the query string
// regardless of the verb: a POST carrying a polluted query is refused the same
// way a GET is.
func TestStrictQueryCoversEveryMethod(t *testing.T) {
	status, body := strictQueryResult(t, http.MethodPost, "box_id=a&box_id=b")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "duplicate query parameter: box_id")
}

// TestStrictQueryRefusalUsesEnvelope pins that refusals go through the shared
// response envelope rather than a bare status, so clients parse them like any
// other framework refusal.
func TestStrictQueryRefusalUsesEnvelope(t *testing.T) {
	_, body := strictQueryResult(t, http.MethodGet, "a=1&a=2")
	require.True(t, strings.Contains(body, `"msg"`) && strings.Contains(body, `"trace_id"`),
		"refusal must use the response envelope, got: %s", body)
}

// BenchmarkStrictQuery prices the gate per request. The parse it performs is
// the request's one memoized parse (the chain parsed the query once before the
// gate existed too), so the admit cases price the gate's own additions: the
// duplicate scan, the read of the page and the size, and the memo write.
func BenchmarkStrictQuery(b *testing.B) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(strictQuery())
	engine.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	cases := []struct {
		name   string
		target string
	}{
		{name: "no query", target: "/probe"},
		{name: "typical list query", target: "/probe?box_id=g1&_page=1&_size=10&status=enabled"},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, c.target, nil)
			b.ReportAllocs()
			for b.Loop() {
				engine.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}
