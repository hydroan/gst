package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/go-playground/validator/v10"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/testutil/swap"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
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

// TestCreateHandlerRestoresNullBodyRequest guards the nil-request contract: a
// literal JSON null body unmarshals into a nil pointer without any binding
// error, and the service must still receive a usable zero-value request.
func TestCreateHandlerRestoresNullBodyRequest(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-null-body-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-null-body-probes", strings.NewReader(`null`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), `"got_nil_request":true`,
		"a JSON null body must reach the service as a zero-value request, not nil")
}

// TestCreateHandlerCompactsNullSliceEntries guards the nil-element contract:
// null entries inside a JSON array must be compacted away before the request
// reaches the service.
func TestCreateHandlerCompactsNullSliceEntries(t *testing.T) {
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

// TestCreateHandlerRejectsTrailingContentAfterJSONBody pins where a request
// body ends: it must be one JSON value and nothing after it. Reading the body
// as a stream stops at the first value and drops whatever follows, so a second
// document — or the tail of a retry appended to the first — would bind as if
// the body had been clean.
func TestCreateHandlerRejectsTrailingContentAfterJSONBody(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-trailing-content-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-trailing-content-probes",
		strings.NewReader(`{"items":[{"name":"first"}]} {"items":[{"name":"second"}]}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code,
		"a body carrying more than one JSON value must be refused, not bound from the first one")
}

// TestCreateHandlerAcceptsTrailingWhitespace keeps the rule above from
// overreaching: whitespace after the body is not content, and bodies written
// with a trailing newline are ordinary.
func TestCreateHandlerAcceptsTrailingWhitespace(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "normalize-trailing-space-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/normalize-trailing-space-probes",
		strings.NewReader("{\"items\":[{\"name\":\"first\"}]}\n  "))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"item_count":1`)
}

// TestCreateHandlerBindFailureNamesOffendingField pins the response envelope
// of a type-mismatch bind failure: a stable message naming the offending field
// through its JSON path, never the decoder's own text with Go struct and
// package internals.
func TestCreateHandlerBindFailureNamesOffendingField(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "bind-error-field-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bind-error-field-probes", strings.NewReader(`{"items":3}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"invalid value for field 'items'"`,
		"a bind failure must render the stable client-safe message, not the raw decoder error")
}

// TestCreateHandlerBindFailureOnMalformedJSON pins the response envelope of a
// syntactically broken body: the generic not-valid-JSON message, with the
// decoder's position details kept for logs only.
func TestCreateHandlerBindFailureOnMalformedJSON(t *testing.T) {
	engine := newNormalizeProbeEngine(t, "bind-error-syntax-probes")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bind-error-syntax-probes", strings.NewReader(`{"items":`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"request body is not valid JSON"`,
		"a malformed body must render the stable client-safe message, not the raw decoder error")
}

// TestCreateHandlerRequiresBodyOnModelPath pins the model-path create
// contract: creating a resource requires a body, so an absent one renders the
// stable required-body rejection instead of a success without a row. The
// delegation path keeps tolerating an empty body — action endpoints without a
// payload live there, pinned by TestCreateHandlerRestoresNullBodyRequest.
func TestCreateHandlerRequiresBodyOnModelPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.POST("/required-body-create-probes",
		CreateHandler[*normalizeProbeModel, *normalizeProbeModel, *normalizeProbeModel]())

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/required-body-create-probes", nil)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"request body is required"`,
		"an absent body must be refused, not answered as an empty success")
}

// TestUpdateHandlerRequiresBody pins the model-path full update contract: an
// absent request body renders the stable required-body message instead of the
// bare io.EOF text.
func TestUpdateHandlerRequiresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.PUT("/required-body-probes/:id",
		UpdateHandler[*normalizeProbeModel, *normalizeProbeModel, *normalizeProbeModel](
			&types.ControllerConfig[*normalizeProbeModel]{Route: "required-body-probes", ParamName: "id"},
		))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/required-body-probes/sample-id", nil)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"request body is required"`,
		"an absent body must render the stable message, not the bare io.EOF text")
}

// TestDeleteManyHandlerBindFailureRendersClientSafeMessage pins the model-path
// batch-delete bind failure to the ordinary invalid-parameter envelope. The
// rendered error must be the bind error itself: this handler once passed a
// separate, still-nil error variable into the envelope, turning every bind
// failure into a nil-dereference panic instead of a 400.
func TestDeleteManyHandlerBindFailureRendersClientSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.DELETE("/bind-error-delete-probes/batch",
		DeleteManyHandler[*normalizeProbeModel, *normalizeProbeModel, *normalizeProbeModel]())

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/bind-error-delete-probes/batch", strings.NewReader(`{"ids":3}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"invalid value for field 'ids'"`)
}

// TestUpdateManyHandlerBindFailureAnswersTheFieldRefused pins the answer of
// a model-path bind failure: 400 with the message naming the field refused,
// aligning the batch and patch handlers with the create/update
// single-resource ones.
func TestUpdateManyHandlerBindFailureAnswersTheFieldRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.PUT("/bind-error-update-probes/batch",
		UpdateManyHandler[*normalizeProbeModel, *normalizeProbeModel, *normalizeProbeModel]())

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/bind-error-update-probes/batch", strings.NewReader(`{"items":3}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"msg":"invalid value for field 'items'"`,
		"a bind failure must name the field refused, not answer as the server's own failure")
}

// validatedProbe carries the binding tags the field-naming tests break: a
// required name, and a required city inside a nested address.
type validatedProbe struct {
	Name    string `json:"name" binding:"required"`
	Address struct {
		City string `json:"city" binding:"required"`
	} `json:"address"`
}

// TestClientSafeBindErrorNamesTheFieldsTheValidatorRefused pins the answer
// to a request the validator refused: 400 with a sentence per field, the
// field named by its JSON key path and the sentences joined by a semicolon,
// each violation readable on the error for the gRPC details, and the item
// of a batch named in front of the field when the item is validated on its
// own.
func TestClientSafeBindErrorNamesTheFieldsTheValidatorRefused(t *testing.T) {
	refused := validateRequest(&validatedProbe{})
	require.Error(t, refused)

	wrapped := clientSafeBindError(refused)
	var serviceErr *types.Error
	require.ErrorAs(t, wrapped, &serviceErr)
	require.Equal(t, http.StatusBadRequest, serviceErr.Status())
	require.Equal(t, "name is a required field; address.city is a required field", serviceErr.Msg())
	require.Equal(t, []types.FieldViolation{
		{Field: "name", Description: "name is a required field"},
		{Field: "address.city", Description: "address.city is a required field"},
	}, types.FieldViolations(serviceErr))
	var cause, direct validator.ValidationErrors
	require.ErrorAs(t, wrapped, &cause, "the validator's error travels as the cause")
	require.ErrorAs(t, refused, &direct)
	require.Equal(t, direct, cause)

	require.ErrorAs(t, clientSafeItemBindError(1, refused), &serviceErr)
	require.Equal(t, "items[1].name is a required field; items[1].address.city is a required field", serviceErr.Msg())
	require.Equal(t, "items[1].name", types.FieldViolations(serviceErr)[0].Field)
}

// untranslatedProbe carries rules the validator has no English sentence for:
// hostname and startswith are two of its own without one, gstprobe is a rule
// a project registers.
type untranslatedProbe struct {
	Host    string `json:"host" binding:"hostname"`
	Address struct {
		Zip string `json:"zip" binding:"hostname"`
	} `json:"address"`
	Tag string `json:"tag" binding:"startswith=ab"`
	Own string `json:"own" binding:"gstprobe"`
}

// TestClientSafeBindErrorSpeaksOfTheRuleWithoutATranslation pins the sentence
// of a field refused by a rule the validator has no English sentence for,
// one of its own or one a project registered: the field's path and the rule,
// with its parameter when it has one, and nothing of the validator's own
// text, which names the Go type of the request.
func TestClientSafeBindErrorSpeaksOfTheRuleWithoutATranslation(t *testing.T) {
	require.NoError(t, validatorEngine.RegisterValidation("gstprobe", func(validator.FieldLevel) bool { return false }))
	refused := validateRequest(&untranslatedProbe{})
	require.Error(t, refused)

	var serviceErr *types.Error
	require.ErrorAs(t, clientSafeBindError(refused), &serviceErr)
	require.Equal(t, []types.FieldViolation{
		{Field: "host", Description: "host failed the hostname check"},
		{Field: "address.zip", Description: "address.zip failed the hostname check"},
		{Field: "tag", Description: "tag failed the startswith=ab check"},
		{Field: "own", Description: "own failed the gstprobe check"},
	}, types.FieldViolations(serviceErr))
	require.NotContains(t, serviceErr.Msg(), "Key:")
	require.NotContains(t, serviceErr.Msg(), "untranslatedProbe")
}

// probeAudit, probeTaggedAudit and probeNamedAudit are the structs
// jsonPathProbe holds in the ways a request struct holds one; ProbeCode is
// the string it embeds.
type (
	probeAudit struct {
		Reviewer string `json:"reviewer" binding:"required"`
	}
	probeTaggedAudit struct {
		Signer string `json:"signer" binding:"required"`
	}
	probeNamedAudit struct {
		Approver string `json:"approver" binding:"required"`
	}
	ProbeCode string
)

// jsonPathProbe holds a struct in each way JSON names, or does not name,
// the level: embedded without a name, whose fields JSON promotes; embedded
// under a name; a named field; a named field without a json tag, which
// JSON names after the field; and the items of a slice. It embeds a string
// too, which JSON names after its type, as a field of its own.
type jsonPathProbe struct {
	probeAudit
	probeTaggedAudit `json:"audit"`
	ProbeCode        `binding:"required"`
	Named            probeNamedAudit `json:"named"`
	Plain            probeNamedAudit
	Items            []probeAudit `json:"items" binding:"dive"`
	Name             string       `json:"name" binding:"required"`
}

// TestClientSafeBindErrorNamesTheFieldsByTheirJSONPath pins that a field is
// named by the path the client sent it under: an embedded struct without a
// name of its own is no level of the path, since JSON promotes its fields,
// where an embedded struct under a name, a named field and a field without
// a json tag each are, under their JSON name, and the items of a slice by
// their index; an embedded value that is no struct is a field under its
// type's name, since JSON promotes nothing of it.
func TestClientSafeBindErrorNamesTheFieldsByTheirJSONPath(t *testing.T) {
	refused := validateRequest(&jsonPathProbe{Items: []probeAudit{{}, {}}})
	require.Error(t, refused)

	var serviceErr *types.Error
	require.ErrorAs(t, clientSafeBindError(refused), &serviceErr)
	fields := make([]string, 0, len(types.FieldViolations(serviceErr)))
	for _, v := range types.FieldViolations(serviceErr) {
		fields = append(fields, v.Field)
	}
	require.Equal(t, []string{"reviewer", "audit.signer", "ProbeCode", "named.approver", "Plain.approver", "items[0].reviewer", "items[1].reviewer", "name"}, fields)
	require.Equal(t, "reviewer is a required field", types.FieldViolations(serviceErr)[0].Description)
	require.Equal(t, "ProbeCode is a required field", types.FieldViolations(serviceErr)[2].Description)
}

// freshPatchHelper marks the child process of
// TestPatchNamesTheFieldInAFreshProcess.
const freshPatchHelper = "GST_TEST_FRESH_PATCH"

// TestPatchNamesTheFieldInAFreshProcess pins that the validator is set up
// before any validation runs, at package initialization: a process whose
// first validation is a patch's, which checks the fields the body names
// alone, names the field refused all the same. The test binary runs itself
// with this test selected and the helper marked, so nothing else validates
// first.
func TestPatchNamesTheFieldInAFreshProcess(t *testing.T) {
	if os.Getenv(freshPatchHelper) == "1" {
		refused := validatePatchFields(&validatedProbe{}, patchFieldSet{"Name": {}})
		require.Error(t, refused)
		var serviceErr *types.Error
		require.ErrorAs(t, clientSafeBindError(refused), &serviceErr)
		require.Equal(t, "name is a required field", serviceErr.Msg())
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestPatchNamesTheFieldInAFreshProcess$", "-test.v")
	cmd.Env = append(os.Environ(), freshPatchHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestPatchNamesTheFieldInAFreshProcess", "the child must have run the helper:\n%s", out)
}

// TestBindJSONRequestHonorsDisabledValidator pins gin's validator-disable
// convention: an application may turn validation off by setting
// binding.Validator to nil, and gin's own binding paths treat that as "skip
// validation". Binding here must do the same instead of dereferencing the
// nil interface and panicking on every request body.
func TestBindJSONRequestHonorsDisabledValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	swap.Value(t, &binding.Validator, nil)

	req := httptest.NewRequest(http.MethodPost, "/bind-probes",
		strings.NewReader(`{"items":[{"name":"first"}]}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	target := &normalizeProbeReq{}
	require.NoError(t, bindJSONRequest(c, target))
	require.Len(t, target.Items, 1)
}

// TestBindJSONRequestDecodesWithStandardLibrary pins request decoding to
// encoding/json whatever JSON codec gin was built with: the body binds as
// encoding/json binds it, and a type mismatch still names the offending field,
// which only encoding/json's error type carries.
func TestBindJSONRequestDecodesWithStandardLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	swap.Value(t, &ginjson.API, ginjson.Core(swappedGinCodec{}))

	bind := func(body string) (*normalizeProbeReq, error) {
		req := httptest.NewRequest(http.MethodPost, "/bind-probes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		target := &normalizeProbeReq{}
		return target, bindJSONRequest(c, target)
	}

	target, err := bind(`{"items":[{"name":"first"}]}`)
	require.NoError(t, err)
	require.Len(t, target.Items, 1)

	_, err = bind(`{"items":3}`)
	var serviceErr *types.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "invalid value for field 'items'", serviceErr.Msg())
}

// swappedGinCodec stands in for the codec gin compiles in under the jsoniter,
// go_json or sonic build tags: decoding through it fails recognizably, and the
// methods it leaves to the nil embedded Core panic when called.
type swappedGinCodec struct{ ginjson.Core }

func (swappedGinCodec) Unmarshal([]byte, any) error { return errors.New("swapped codec") }

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

// TestClientSafeBindError pins the translation table of body decoding
// failures: one stable client-safe message per decoder error kind, a field
// inside a batch item named as the contract spells it, items[1].rank, with
// the original error preserved as the cause so logs keep the full decoder
// text.
func TestClientSafeBindError(t *testing.T) {
	// batchProbe carries an integer narrower than a JSON number inside its
	// items, for a value a field of an item cannot hold.
	type batchProbe struct {
		Items []struct {
			Rank int8 `json:"rank"`
		} `json:"items"`
	}
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"type_mismatch_names_the_field", json.Unmarshal([]byte(`{"items":3}`), &normalizeProbeReq{}), "invalid value for field 'items'"},
		{"type_mismatch_inside_an_item_names_the_item", json.Unmarshal([]byte(`{"items":[{"rank":1},{"rank":300}]}`), &batchProbe{}), "invalid value for field 'items[1].rank'"},
		{"top-level_type_mismatch_has_no_field", json.Unmarshal([]byte(`[1]`), &normalizeProbeReq{}), "request body has an unexpected JSON type"},
		{"malformed_body", json.Unmarshal([]byte(`{`), &normalizeProbeReq{}), "request body is not valid JSON"},
		{"other_errors_fall_back_to_the_generic_message", errors.New("read failed"), "invalid request body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := clientSafeBindError(tt.err)

			var serviceErr *types.Error
			require.ErrorAs(t, wrapped, &serviceErr)
			require.Equal(t, tt.wantMsg, serviceErr.Msg())
			require.Equal(t, http.StatusBadRequest, serviceErr.Status())
			require.ErrorIs(t, wrapped, tt.err, "the original error must survive as the cause")
			require.Contains(t, wrapped.Error(), tt.err.Error(), "logs must keep the full decoder text")
		})
	}
}

// TestNormalizeValueCompactsNilSliceElements covers the reflective walk over
// the value shapes JSON binding can produce.
func TestNormalizeValueCompactsNilSliceElements(t *testing.T) {
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
	normalizeValue(reflect.ValueOf(s))

	require.Equal(t, []*normalizeProbeItem{first, second}, s.Items, "top-level slice keeps order without nils")
	require.Equal(t, []*normalizeProbeItem{first}, s.Nested.Records, "slices inside nested structs are compacted")
	require.Equal(t, []*normalizeProbeItem{second}, s.Chained.Records, "slices behind pointer chains are compacted")
	require.Equal(t, []*normalizeProbeItem{first}, s.ByKey["only"], "slices held as map values are compacted")
	require.Equal(t, []string{"kept", "", "kept-too"}, s.Names, "slices of non-nilable elements stay untouched")
	require.Nil(t, s.Missing, "nil slices stay nil instead of becoming empty")
}

// TestNormalizeValueReadsDatesInUTC covers the date half of the walk: every
// datatypes.Date it can set — a field, behind a pointer, in a slice, held as
// a map value, inside a nested struct — becomes the UTC day of the instant
// at midnight, the zero date stays zero, and a time.Time, an instant, is
// left as it is.
func TestNormalizeValueReadsDatesInUTC(t *testing.T) {
	type inner struct {
		Day datatypes.Date `json:"day"`
	}
	type sample struct {
		Day     datatypes.Date            `json:"day"`
		Ptr     *datatypes.Date           `json:"ptr"`
		Days    []datatypes.Date          `json:"days"`
		ByKey   map[string]datatypes.Date `json:"by_key"`
		Nested  inner                     `json:"nested"`
		Zero    datatypes.Date            `json:"zero"`
		Instant time.Time                 `json:"instant"`
	}

	shanghai := time.FixedZone("CST", 8*60*60)
	sent := time.Date(2026, time.January, 2, 0, 0, 0, 0, shanghai)
	ptr := datatypes.Date(sent)
	s := &sample{
		Day:     datatypes.Date(sent),
		Ptr:     &ptr,
		Days:    []datatypes.Date{datatypes.Date(sent)},
		ByKey:   map[string]datatypes.Date{"only": datatypes.Date(sent)},
		Nested:  inner{Day: datatypes.Date(sent)},
		Instant: sent,
	}
	normalizeValue(reflect.ValueOf(s))

	want := datatypes.Date(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	require.Equal(t, want, s.Day)
	require.Equal(t, want, *s.Ptr)
	require.Equal(t, []datatypes.Date{want}, s.Days)
	require.Equal(t, map[string]datatypes.Date{"only": want}, s.ByKey)
	require.Equal(t, want, s.Nested.Day)
	require.True(t, time.Time(s.Zero).IsZero(), "the zero date stays zero")
	require.True(t, s.Instant.Equal(sent), "an instant is not a calendar date and stays as sent")
	require.Equal(t, shanghai, s.Instant.Location())
}

// newNormalizeProbeEngine wires the probe service into a fresh engine on its
// own route so each test observes exactly what the service receives.
func newNormalizeProbeEngine(t *testing.T, route string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	registerTestService[*normalizeProbeModel, *normalizeProbeReq, *normalizeProbeRsp](consts.Create, route, &normalizeProbeService{})
	engine := gin.New()
	engine.POST("/"+route, CreateHandler[*normalizeProbeModel, *normalizeProbeReq, *normalizeProbeRsp](&types.ControllerConfig[*normalizeProbeModel]{Route: route}))
	return engine
}
