package openapigen

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/config"
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

// DocumentHandler returns an http.Handler that serves the OpenAPI document.
// The document is built on the first request rather than at route registration,
// so a process that never serves it never pays for building it.
func DocumentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		build()

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

// setDocSecurity declares the authentication schemes the project's
// middleware registered (see apidoc.RegisterSecurityScheme), by name, and
// requires any one of them of every operation by default; public operations
// override this with an empty requirement (see markPublic). A project that
// registered none, serving no authentication, declares none and requires
// nothing.
func setDocSecurity(doc *openapi3.T) {
	schemes := apidoc.SecuritySchemes()
	if len(schemes) == 0 {
		return
	}
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = openapi3.SecuritySchemes{}
	}
	doc.Security = openapi3.SecurityRequirements{}
	for _, name := range slices.Sorted(maps.Keys(schemes)) {
		scheme := schemes[name]
		doc.Components.SecuritySchemes[name] = &openapi3.SecuritySchemeRef{
			Value: &openapi3.SecurityScheme{
				Type:        scheme.Type,
				Scheme:      scheme.Scheme,
				In:          scheme.In,
				Name:        scheme.Name,
				Description: scheme.Description,
			},
		}
		doc.Security = append(doc.Security, openapi3.SecurityRequirement{name: []string{}})
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
