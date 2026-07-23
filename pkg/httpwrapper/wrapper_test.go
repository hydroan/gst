package httpwrapper

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrappedRequest(t *testing.T) {
	t.Run("json body", func(t *testing.T) {
		body := []byte(`{"name":"sample"}`)
		req, err := http.NewRequest(http.MethodPost, "http://sample.local", bytes.NewReader(body))
		require.NoError(t, err)
		testWrappedRequest(t, req, body)
	})

	t.Run("empty body", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, "http://sample.local", bytes.NewReader([]byte{}))
		require.NoError(t, err)
		testWrappedRequest(t, req, []byte{})
	})

	t.Run("nil body", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://sample.local", nil)
		require.NoError(t, err)
		testWrappedRequest(t, req, []byte{})
	})
}

func TestWrappedResponse(t *testing.T) {
	body := []byte(`{"name":"sample"}`)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	t.Run("http", func(t *testing.T) {
		srv := httptest.NewServer(handler)
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		testWrappedResponse(t, resp, body)
	})

	t.Run("https", func(t *testing.T) {
		srv := httptest.NewTLSServer(handler)
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.NotNil(t, resp.TLS)
		testWrappedResponse(t, resp, body)
	})
}

// testWrappedRequest verifies that req survives a JSON marshal/unmarshal
// round trip with its body restored to the expected bytes.
func testWrappedRequest(t *testing.T, req *http.Request, body []byte) {
	t.Helper()

	reqWrapper := &WrappedRequest{Request: req}
	data, err := json.Marshal(reqWrapper)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	reqWrapper = new(WrappedRequest)
	require.NoError(t, json.Unmarshal(data, reqWrapper))
	got, err := io.ReadAll(reqWrapper.Body)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

// testWrappedResponse verifies that resp survives a JSON marshal/unmarshal
// round trip with its body and TLS state restored.
func testWrappedResponse(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()

	respWrapper := &WrappedResponse{Response: resp}
	data, err := json.Marshal(respWrapper)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	respWrapper = new(WrappedResponse)
	require.NoError(t, json.Unmarshal(data, respWrapper))
	got, err := io.ReadAll(respWrapper.Body)
	require.NoError(t, err)
	assert.Equal(t, body, got)

	if resp.TLS != nil {
		require.NotNil(t, respWrapper.TLS)
		require.Len(t, respWrapper.TLS.PeerCertificates, len(resp.TLS.PeerCertificates))
		for i, cert := range resp.TLS.PeerCertificates {
			assert.Equal(t, cert.Raw, respWrapper.TLS.PeerCertificates[i].Raw)
		}
	}
}
