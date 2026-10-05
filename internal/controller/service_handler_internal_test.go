package controller

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// payloadReq and payloadRsp are the payload and result of the sample's own
// actions: what the service handler binds and answers with.
type (
	payloadReq struct {
		Note string `json:"note"`
	}
	payloadRsp struct {
		Note string `json:"note"`
	}
)

// payloadService answers every action with the note it was given, so a test
// reads back what the handler bound.
type payloadService struct {
	serviceregistry.Base[*handlerRouteModel, *payloadReq, *payloadRsp]
}

// requiredReq is a payload with a required field, which the zero request of
// an absent body fails to meet; requiredService answers with its note.
type requiredReq struct {
	Note string `json:"note" binding:"required"`
}

type requiredService struct {
	serviceregistry.Base[*handlerRouteModel, *requiredReq, *payloadRsp]
}

func (*requiredService) Create(_ *types.ServiceContext, req *requiredReq) (*payloadRsp, error) {
	return &payloadRsp{Note: req.Note}, nil
}

func (*payloadService) Create(_ *types.ServiceContext, req *payloadReq) (*payloadRsp, error) {
	return &payloadRsp{Note: req.Note}, nil
}

func (*payloadService) Get(_ *types.ServiceContext, req *payloadReq) (*payloadRsp, error) {
	return &payloadRsp{Note: req.Note}, nil
}

func (*payloadService) List(_ *types.ServiceContext, req *payloadReq) (*payloadRsp, error) {
	return &payloadRsp{Note: req.Note}, nil
}

// TestServiceHandlerValidatesTheZeroRequestOfAnAbsentBody pins that an
// absent body, a JSON null included, is bound as the zero request and
// validated all the same, the way the gRPC call validates a payload the
// message left unset: a required field refuses it with 400 where a body
// naming the field passes. A payload without required fields still reaches
// the service empty (see TestServiceHandlerBindsWhatTheActionReads).
func TestServiceHandlerValidatesTheZeroRequestOfAnAbsentBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "required-payloads"
	registerTestService[*handlerRouteModel, *requiredReq, *payloadRsp](consts.Create, route, &requiredService{})
	cfg := &types.ControllerConfig[*handlerRouteModel]{Route: route, ParamName: "id"}
	engine := gin.New()
	engine.POST("/required-payloads", CreateHandler[*handlerRouteModel, *requiredReq, *payloadRsp](cfg))

	for _, tt := range []struct {
		name   string
		body   string
		status int
	}{
		{name: "an absent body fails the required field", status: http.StatusBadRequest},
		{name: "a JSON null fails the required field", body: "null", status: http.StatusBadRequest},
		{name: "a body naming the field passes", body: `{"note":"named"}`, status: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/required-payloads", strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
		})
	}
}

// TestServiceHandlerBindsWhatTheActionReads pins the one rule of the
// handler of an action with a payload of its own: the JSON body is bound
// into the payload, an absent body leaving it zero and a malformed one
// refused, except for a Create sent as a multipart form, whose body is left
// for the service, and for a Get or List, whose GET request carries no body
// however much is sent.
func TestServiceHandlerBindsWhatTheActionReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "payloads"
	for _, phase := range []consts.Phase{consts.Create, consts.Get, consts.List} {
		registerTestService[*handlerRouteModel, *payloadReq, *payloadRsp](phase, route, &payloadService{})
	}
	cfg := &types.ControllerConfig[*handlerRouteModel]{Route: route, ParamName: "id"}
	engine := gin.New()
	engine.POST("/payloads", CreateHandler[*handlerRouteModel, *payloadReq, *payloadRsp](cfg))
	engine.GET("/payloads/:id", GetHandler[*handlerRouteModel, *payloadReq, *payloadRsp](cfg))
	engine.GET("/payloads", ListHandler[*handlerRouteModel, *payloadReq, *payloadRsp](cfg))

	form := new(bytes.Buffer)
	writer := multipart.NewWriter(form)
	require.NoError(t, writer.WriteField("note", "in the form"))
	require.NoError(t, writer.Close())

	for _, tt := range []struct {
		name        string
		method      string
		target      string
		body        string
		contentType string
		status      int
		note        string
	}{
		{name: "a create binds the body", method: http.MethodPost, target: "/payloads", body: `{"note":"bound"}`, contentType: "application/json", status: http.StatusOK, note: "bound"},
		{name: "a create without a body binds nothing", method: http.MethodPost, target: "/payloads", status: http.StatusOK},
		{name: "a create with a malformed body is refused", method: http.MethodPost, target: "/payloads", body: `{"note":`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "a create sent as a form is left to the service", method: http.MethodPost, target: "/payloads", body: form.String(), contentType: writer.FormDataContentType(), status: http.StatusOK},
		{name: "a get binds nothing", method: http.MethodGet, target: "/payloads/1", body: `{"note":"sent"}`, contentType: "application/json", status: http.StatusOK},
		{name: "a list binds nothing", method: http.MethodGet, target: "/payloads?note=sent", body: `{"note":"sent"}`, contentType: "application/json", status: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
			if tt.status != http.StatusOK {
				return
			}
			var envelope struct {
				Data payloadRsp `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
			require.Equal(t, tt.note, envelope.Data.Note)
		})
	}
}
