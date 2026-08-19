package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type patchValueTestRecord struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Enabled bool   `json:"enabled"`
}

func TestHandleServiceErrorDoesNotExposeCause(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	cause := errors.New("database password leaked")

	handleServiceError(ctx, serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.JSONEq(t, `{"code":-1,"msg":"failed to load user","data":null,"trace_id":""}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), cause.Error())
}

func TestHandleServiceErrorUsesServiceErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	handleServiceError(ctx, serviceregistry.NewError(http.StatusForbidden, "account disabled"))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, -1, body.Code)
	require.Equal(t, "account disabled", body.Msg)
}

func TestPatchValueAppliesExplicitZeroValues(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()
	oldRecord := &patchValueTestRecord{
		Name:    "enabled feature",
		Count:   10,
		Enabled: true,
	}
	newRecord := &patchValueTestRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}

	patchValue(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem())

	require.Empty(t, oldRecord.Name)
	require.Zero(t, oldRecord.Count)
	require.False(t, oldRecord.Enabled)
}

func TestPatchValueSkipsMissingFields(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()
	oldRecord := &patchValueTestRecord{
		Name:    "enabled feature",
		Count:   10,
		Enabled: true,
	}
	newRecord := &patchValueTestRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}

	patchValue(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem(), patchFieldSet{
		"Enabled": {},
	})

	require.Equal(t, "enabled feature", oldRecord.Name)
	require.Equal(t, 10, oldRecord.Count)
	require.False(t, oldRecord.Enabled)
}

type routeIDUUIDRecord struct {
	Name string `json:"name"`

	modelregistry.Base
}

type routeIDIntegerRecord struct {
	Name string `json:"name"`

	modelregistry.AutoBase
}

func TestSetRouteIDAcceptsAnyValueForUUIDKeyedModel(t *testing.T) {
	m := new(routeIDUUIDRecord)

	require.True(t, setRouteID(m, "custom-id"))
	require.Equal(t, "custom-id", m.GetID())
}

func TestSetRouteIDNormalizesIntegerKeyedModelID(t *testing.T) {
	m := new(routeIDIntegerRecord)

	require.True(t, setRouteID(m, "007"))
	require.Equal(t, "7", m.GetID())
	require.Equal(t, uint64(7), m.ID)
}

func TestSetRouteIDRejectsUnparsableIntegerKeyedModelID(t *testing.T) {
	for _, id := range []string{"abc", "7abc", "0", "-1", "18446744073709551616"} {
		m := new(routeIDIntegerRecord)

		require.Falsef(t, setRouteID(m, id), "id %q should be rejected", id)
		require.Zero(t, m.ID)
	}
}

func TestPatchFieldSetFromJSONBodyUsesJSONTags(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":false,"count":0}`))

	require.NoError(t, err)
	require.Contains(t, fields, "Enabled")
	require.Contains(t, fields, "Count")
	require.NotContains(t, fields, "Name")
}

func TestPatchManyFieldSetsFromJSONBodyKeepItemFieldsSeparate(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()

	fieldSets, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":[{"enabled":false},{"name":""}]}`))

	require.NoError(t, err)
	require.Len(t, fieldSets, 2)
	require.Contains(t, fieldSets[0], "Enabled")
	require.NotContains(t, fieldSets[0], "Name")
	require.Contains(t, fieldSets[1], "Name")
	require.NotContains(t, fieldSets[1], "Enabled")
}

// TestHandleServiceErrorHidesInternalErrorText pins the fallback branch: an
// error that is not a service-layer error renders the generic failure message,
// keeping driver and infrastructure text out of the envelope.
func TestHandleServiceErrorHidesInternalErrorText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	internal := errors.New("dial tcp 10.0.0.1:3306: connection refused")

	handleServiceError(ctx, internal)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"code":-1,"msg":"failure","data":null,"trace_id":""}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), internal.Error())
}

// TestDatabaseErrorCoder pins the canonical mapping of database errors: a
// service error keeps its own status and message, the two database sentinels
// render their fixed codes, and everything else falls back to the generic
// failure message without carrying internal error text.
func TestDatabaseErrorCoder(t *testing.T) {
	serviceErr := serviceregistry.NewError(http.StatusForbidden, "operation refused")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"service error keeps status and message", serviceErr, http.StatusForbidden, "operation refused"},
		{"record not found renders 404", errors.Wrap(database.ErrRecordNotFound, "get sample"), http.StatusNotFound, "Requested resource not found."},
		{"duplicated key renders 409", errors.Wrap(database.ErrDuplicatedKey, "create sample"), http.StatusConflict, "Resource already exists."},
		{"other errors hide internal text", errors.New("Error 1146: Table 'sample' doesn't exist"), http.StatusBadRequest, "failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coder := databaseErrorCoder(tt.err)

			require.Equal(t, tt.wantStatus, coder.Status())
			require.Equal(t, tt.wantMsg, coder.Msg())
		})
	}
}

// TestPatchFieldSetFromJSONBodyWrapsDecodeError pins that patch-path body
// decoding failures carry the client-safe message instead of the raw decoder
// text, matching the bindJSONRequest contract.
func TestPatchFieldSetFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()

	_, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":`))

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "request body is not valid JSON", serviceErr.Msg())
}

// TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError is the batch-path
// counterpart: a type mismatch on the items envelope must name the field
// through the client-safe message.
func TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchValueTestRecord]()

	_, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":3}`))

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "invalid value for field 'items'", serviceErr.Msg())
}

func BenchmarkPatchValueModelPatch(b *testing.B) {
	typ := reflect.TypeFor[patchValueTestRecord]()
	newRecord := &patchValueTestRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}
	newVal := reflect.ValueOf(newRecord).Elem()
	log := nopControllerLogger{}
	fields := patchFieldSet{
		"Name":    {},
		"Count":   {},
		"Enabled": {},
	}

	b.ReportAllocs()
	for range b.N {
		oldRecord := &patchValueTestRecord{
			Name:    "enabled feature",
			Count:   10,
			Enabled: true,
		}
		patchValue(log, typ, reflect.ValueOf(oldRecord).Elem(), newVal, fields)
	}
}

type nopControllerLogger struct{}

func (nopControllerLogger) With(fields ...string) types.Logger { return nopControllerLogger{} }
func (nopControllerLogger) WithContext(context.Context, consts.Phase) types.Logger {
	return nopControllerLogger{}
}
func (nopControllerLogger) Debug(args ...any)                       {}
func (nopControllerLogger) Info(args ...any)                        {}
func (nopControllerLogger) Warn(args ...any)                        {}
func (nopControllerLogger) Error(args ...any)                       {}
func (nopControllerLogger) Fatal(args ...any)                       {}
func (nopControllerLogger) Debugf(format string, args ...any)       {}
func (nopControllerLogger) Infof(format string, args ...any)        {}
func (nopControllerLogger) Warnf(format string, args ...any)        {}
func (nopControllerLogger) Errorf(format string, args ...any)       {}
func (nopControllerLogger) Fatalf(format string, args ...any)       {}
func (nopControllerLogger) Debugw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Infow(msg string, keysAndValues ...any)  {}
func (nopControllerLogger) Warnw(msg string, keysAndValues ...any)  {}
func (nopControllerLogger) Errorw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Fatalw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Debugz(msg string, fields ...zap.Field)  {}
func (nopControllerLogger) Infoz(msg string, fields ...zap.Field)   {}
func (nopControllerLogger) Warnz(msg string, fields ...zap.Field)   {}
func (nopControllerLogger) Errorz(msg string, fields ...zap.Field)  {}
func (nopControllerLogger) Fatalz(msg string, fields ...zap.Field)  {}
