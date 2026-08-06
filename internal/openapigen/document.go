package openapigen

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/config"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
)

var (
	doc = &openapi3.T{
		OpenAPI: "3.0.0",
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas:       openapi3.Schemas{},
			RequestBodies: openapi3.RequestBodies{},
			Responses:     openapi3.ResponseBodies{},
		},
	}
	// docMutex protects concurrent access to the global doc variable
	docMutex sync.RWMutex
)

func Write(filename string) error {
	docMutex.RLock()
	data, err := json.MarshalIndent(doc, "", "  ")
	docMutex.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o600)
}

// DocumentHandler returns an http.Handler that serves the OpenAPI document
func DocumentHandler() http.Handler {
	docMutex.Lock()
	setDocInfo(doc)
	docMutex.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		docMutex.RLock()
		data, _ := json.Marshal(doc)
		docMutex.RUnlock()
		_, _ = w.Write(data)
	})
}

// unversionedAppVersion stands in for an application that never declares a
// version. Leaving the app version unset is normal, while the spec requires
// info.version to be a non-empty string.
const unversionedAppVersion = "0.0.0"

func setDocInfo(doc *openapi3.T) {
	version := config.App.AppInfo.Version
	if version == "" {
		version = unversionedAppVersion
	}
	doc.Info = &openapi3.Info{
		Title:       config.App.AppInfo.Name,
		Description: config.App.AppInfo.Name + " Restful api docs",
		Version:     version,
	}
	setDocSecurity(doc)
}

// The component names of the supported authentication mechanisms: the IAM
// session cookie and the bearer token.
const (
	securitySchemeCookie = "cookieAuth"
	securitySchemeBearer = "bearerAuth"
)

// setDocSecurity declares the authentication mechanisms and requires one of
// them for every operation by default. Public operations override this with
// an empty security requirement (see markPublic).
func setDocSecurity(doc *openapi3.T) {
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = openapi3.SecuritySchemes{}
	}
	doc.Components.SecuritySchemes[securitySchemeCookie] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "cookie",
			Name:        serviceiamsession.SessionCookieName,
			Description: "IAM session cookie issued by POST /api/login",
		},
	}
	doc.Components.SecuritySchemes[securitySchemeBearer] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:   "http",
			Scheme: "bearer",
		},
	}
	doc.Security = openapi3.SecurityRequirements{
		{securitySchemeCookie: []string{}},
		{securitySchemeBearer: []string{}},
	}
}

// markPublic documents an operation as accessible without authentication by
// overriding the document-level security with an empty requirement list.
func markPublic(op *openapi3.Operation) {
	if op == nil {
		return
	}
	op.Security = &openapi3.SecurityRequirements{}
}
