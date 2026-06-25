package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/service"
	"github.com/stretchr/testify/require"
)

func TestHandleServiceErrorDoesNotExposeCause(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	cause := errors.New("database password leaked")

	handleServiceError(ctx, nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.JSONEq(t, `{"code":-1,"msg":"failed to load user","data":null,"trace_id":""}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), cause.Error())
}

func TestHandleServiceErrorUsesServiceErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	handleServiceError(ctx, nil, service.NewError(http.StatusForbidden, "account disabled"))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, -1, body.Code)
	require.Equal(t, "account disabled", body.Msg)
}
