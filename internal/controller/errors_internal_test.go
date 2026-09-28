package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
)

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
	require.JSONEq(t, `{"code":-1,"msg":"The request could not be processed.","data":null,"trace_id":""}`, recorder.Body.String())
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
		{"service_error_keeps_status_and_message", serviceErr, http.StatusForbidden, "operation refused"},
		{"record_not_found_renders_404", errors.Wrap(database.ErrRecordNotFound, "get sample"), http.StatusNotFound, "The requested resource was not found."},
		{"duplicated_key_renders_409", errors.Wrap(database.ErrDuplicatedKey, "create sample"), http.StatusConflict, "The resource already exists."},
		{"stale_object_renders_409", errors.Wrap(database.ErrStaleObject, "update sample"), http.StatusConflict, "The resource was modified by another operation. Reload and retry."},
		{"missing_version_renders_400", errors.Wrap(database.ErrVersionRequired, "update sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"missing_id_renders_400", errors.Wrap(database.ErrIDRequired, "update sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"other_errors_hide_internal_text", errors.New("Error 1146: Table 'sample' doesn't exist"), http.StatusBadRequest, "The request could not be processed."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coder := databaseErrorCoder(tt.err)

			require.Equal(t, tt.wantStatus, coder.Status())
			require.Equal(t, tt.wantMsg, coder.Msg())
		})
	}
}
