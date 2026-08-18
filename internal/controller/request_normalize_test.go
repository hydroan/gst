package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// normalizeProbeModel is the model fixture of the request-normalization tests.
type normalizeProbeModel struct {
	modelregistry.Base
}

type normalizeProbeItem struct {
	Name string `json:"name"`
}

type normalizeProbeReq struct {
	Items []*normalizeProbeItem `json:"items"`
}

// normalizeProbeRsp reports what the service actually observed, so the tests
// can assert on the request shape after controller-side normalization.
type normalizeProbeRsp struct {
	GotNilRequest bool `json:"got_nil_request"`
	ItemCount     int  `json:"item_count"`
	GotNilItem    bool `json:"got_nil_item"`
}

type normalizeProbeService struct {
	serviceregistry.Base[*normalizeProbeModel, *normalizeProbeReq, *normalizeProbeRsp]
}

func (s *normalizeProbeService) Create(_ *types.ServiceContext, req *normalizeProbeReq) (*normalizeProbeRsp, error) {
	if req == nil {
		return &normalizeProbeRsp{GotNilRequest: true}, nil
	}
	rsp := &normalizeProbeRsp{ItemCount: len(req.Items)}
	for _, item := range req.Items {
		if item == nil {
			rsp.GotNilItem = true
		}
	}
	return rsp, nil
}

// newNormalizeProbeEngine wires the probe service into a fresh engine on its
// own route so each test observes exactly what the service receives.
func newNormalizeProbeEngine(t *testing.T, route string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger.Controller = zap.New("")

	serviceregistry.Register[*normalizeProbeModel, *normalizeProbeReq, *normalizeProbeRsp](consts.PHASE_CREATE, route, &normalizeProbeService{})
	engine := gin.New()
	engine.POST("/"+route, CreateFactory[*normalizeProbeModel, *normalizeProbeReq, *normalizeProbeRsp](&types.ControllerConfig[*normalizeProbeModel]{Route: route}))
	return engine
}

// TestCreateFactoryRestoresNullBodyRequest guards the nil-request contract: a
// literal JSON null body unmarshals into a nil pointer without any binding
// error, and the service must still receive a usable zero-value request.
func TestCreateFactoryRestoresNullBodyRequest(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-null-body-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-null-body-probes", strings.NewReader(`null`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), `"got_nil_request":true`,
		"a JSON null body must reach the service as a zero-value request, not nil")
}

// TestCreateFactoryCompactsNullSliceEntries guards the nil-element contract:
// null entries inside a JSON array must be compacted away before the request
// reaches the service.
func TestCreateFactoryCompactsNullSliceEntries(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-null-item-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-null-item-probes",
		strings.NewReader(`{"items":[null,{"name":"first"},null,{"name":"second"}]}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"item_count":2`,
		"null entries must be removed while real items survive")
	require.NotContains(t, recorder.Body.String(), `"got_nil_item":true`,
		"the service must never observe a nil slice element")
}

// TestCreateFactoryRejectsTrailingContentAfterJSONBody pins where a request
// body ends: it must be one JSON value and nothing after it. Reading the body
// as a stream stops at the first value and drops whatever follows, so a second
// document — or the tail of a retry appended to the first — would bind as if
// the body had been clean.
func TestCreateFactoryRejectsTrailingContentAfterJSONBody(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-trailing-content-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-trailing-content-probes",
		strings.NewReader(`{"items":[{"name":"first"}]} {"items":[{"name":"second"}]}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code,
		"a body carrying more than one JSON value must be refused, not bound from the first one")
}

// TestCreateFactoryAcceptsTrailingWhitespace keeps the rule above from
// overreaching: whitespace after the body is not content, and bodies written
// with a trailing newline are ordinary.
func TestCreateFactoryAcceptsTrailingWhitespace(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-trailing-space-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-trailing-space-probes",
		strings.NewReader("{\"items\":[{\"name\":\"first\"}]}\n  "))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"item_count":1`)
}

// TestCreateFactoryBindFailureNamesOffendingField pins the response envelope
// of a type-mismatch bind failure: a stable message naming the offending field
// through its JSON path, never the decoder's own text with Go struct and
// package internals.
func TestCreateFactoryBindFailureNamesOffendingField(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "bind-error-field-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bind-error-field-probes", strings.NewReader(`{"items":3}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"invalid value for field 'items'"`,
		"a bind failure must render the stable client-safe message, not the raw decoder error")
}

// TestCreateFactoryBindFailureOnMalformedJSON pins the response envelope of a
// syntactically broken body: the generic not-valid-JSON message, with the
// decoder's position details kept for logs only.
func TestCreateFactoryBindFailureOnMalformedJSON(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "bind-error-syntax-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bind-error-syntax-probes", strings.NewReader(`{"items":`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"request body is not valid JSON"`,
		"a malformed body must render the stable client-safe message, not the raw decoder error")
}

// TestUpdateManyFactoryBindFailureRendersInvalidParamCode pins the error code
// of a model-path bind failure: every bind failure classifies as invalid
// parameters, aligning the batch and patch handlers with the create/update
// single-resource ones.
func TestUpdateManyFactoryBindFailureRendersInvalidParamCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.Controller = zap.New("")

	engine := gin.New()
	engine.PUT("/bind-error-code-probes/batch",
		UpdateManyFactory[*normalizeProbeModel, *normalizeProbeModel, *normalizeProbeModel]())

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bind-error-code-probes/batch", strings.NewReader(`{"items":3}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":1000`,
		"a bind failure must classify as invalid parameters, not the generic failure code")
	require.Contains(t, recorder.Body.String(), `"msg":"invalid value for field 'items'"`)
}

// TestClientSafeBindError pins the translation table of body decoding
// failures: one stable client-safe message per decoder error kind, with the
// original error preserved as the cause so logs keep the full decoder text.
func TestClientSafeBindError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"type mismatch names the field", json.Unmarshal([]byte(`{"items":3}`), &normalizeProbeReq{}), "invalid value for field 'items'"},
		{"top-level type mismatch has no field", json.Unmarshal([]byte(`[1]`), &normalizeProbeReq{}), "request body has an unexpected JSON type"},
		{"malformed body", json.Unmarshal([]byte(`{`), &normalizeProbeReq{}), "request body is not valid JSON"},
		{"other errors fall back to the generic message", errors.New("read failed"), "invalid request body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := clientSafeBindError(tt.err)

			var serviceErr *serviceregistry.Error
			require.ErrorAs(t, wrapped, &serviceErr)
			require.Equal(t, tt.wantMsg, serviceErr.Msg())
			require.Equal(t, http.StatusBadRequest, serviceErr.Status())
			require.ErrorIs(t, wrapped, tt.err, "the original error must survive as the cause")
			require.Contains(t, wrapped.Error(), tt.err.Error(), "logs must keep the full decoder text")
		})
	}
}

// TestBindJSONRequestHonorsDisabledValidator pins gin's validator-disable
// convention: an application may turn validation off by setting
// binding.Validator to nil, and gin's own binding paths treat that as "skip
// validation". Binding here must do the same instead of dereferencing the
// nil interface and panicking on every request body.
func TestBindJSONRequestHonorsDisabledValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := binding.Validator
	binding.Validator = nil
	t.Cleanup(func() { binding.Validator = restore })

	req := httptest.NewRequest(http.MethodPost, "/bind-probes",
		strings.NewReader(`{"items":[{"name":"first"}]}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	target := &normalizeProbeReq{}
	require.NoError(t, bindJSONRequest(c, target))
	require.Len(t, target.Items, 1)
}

// BenchmarkBindJSONRequest measures what binding one request body costs, the
// price every write endpoint pays before its service sees anything.
func BenchmarkBindJSONRequest(b *testing.B) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"items":[{"name":"first"},{"name":"second"}]}`)

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/bind-probes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req

		target := &normalizeProbeReq{}
		if err := bindJSONRequest(c, &target); err != nil {
			b.Fatal(err)
		}
	}
}

// TestCompactNilSliceElements covers the reflective walk over the value
// shapes JSON binding can produce.
func TestCompactNilSliceElements(t *testing.T) {
	type inner struct {
		Records []*normalizeProbeItem `json:"records"`
	}
	type sample struct {
		Items   []*normalizeProbeItem            `json:"items"`
		Nested  inner                            `json:"nested"`
		Chained *inner                           `json:"chained"`
		ByKey   map[string][]*normalizeProbeItem `json:"by_key"`
		Names   []string                         `json:"names"`
		Missing []*normalizeProbeItem            `json:"missing"`
	}

	first, second := &normalizeProbeItem{Name: "first"}, &normalizeProbeItem{Name: "second"}
	s := &sample{
		Items:   []*normalizeProbeItem{nil, first, nil, second},
		Nested:  inner{Records: []*normalizeProbeItem{nil, first}},
		Chained: &inner{Records: []*normalizeProbeItem{second, nil}},
		ByKey:   map[string][]*normalizeProbeItem{"only": {nil, first, nil}},
		Names:   []string{"kept", "", "kept-too"},
	}
	compactNilSliceElements(reflect.ValueOf(s))

	require.Equal(t, []*normalizeProbeItem{first, second}, s.Items, "top-level slice keeps order without nils")
	require.Equal(t, []*normalizeProbeItem{first}, s.Nested.Records, "slices inside nested structs are compacted")
	require.Equal(t, []*normalizeProbeItem{second}, s.Chained.Records, "slices behind pointer chains are compacted")
	require.Equal(t, []*normalizeProbeItem{first}, s.ByKey["only"], "slices held as map values are compacted")
	require.Equal(t, []string{"kept", "", "kept-too"}, s.Names, "slices of non-nilable elements stay untouched")
	require.Nil(t, s.Missing, "nil slices stay nil instead of becoming empty")
}
