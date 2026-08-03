package testutil

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

func TestRequireErrorAcceptsWrappedRejectionAndReturnsIt(t *testing.T) {
	err := errors.Wrap(&client.Error{
		StatusCode: http.StatusForbidden,
		Code:       -1,
		Msg:        "permission denied for sample",
	}, "call sample endpoint")

	respErr := RequireError(t, err, http.StatusForbidden, "permission denied", "sample")
	require.Equal(t, -1, respErr.Code)
}

func TestDecodeRespReturnsDecodedPayload(t *testing.T) {
	resp := &client.Envelope{
		Code:    0,
		Msg:     "success",
		Data:    json.RawMessage(`{"name":"sample","count":2}`),
		TraceID: "trace-1",
	}

	rsp := DecodeResp[struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}](t, resp)
	require.Equal(t, "sample", rsp.Name)
	require.Equal(t, 2, rsp.Count)
}

func TestRequireDataFieldsSeesTopLevelFields(t *testing.T) {
	resp := &client.Envelope{
		Data: json.RawMessage(`{"enabled":false,"device_count":0}`),
	}

	RequireDataFields(t, resp, "enabled", "device_count")
}
