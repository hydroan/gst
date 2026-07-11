package openapigen

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/types/consts"
)

func TestSetDocSecurityDeclaresSchemes(t *testing.T) {
	testDoc := &openapi3.T{Components: &openapi3.Components{}}
	setDocSecurity(testDoc)

	cookie := testDoc.Components.SecuritySchemes[securitySchemeCookie]
	if cookie == nil || cookie.Value == nil {
		t.Fatal("cookieAuth scheme missing")
	}
	if cookie.Value.Type != "apiKey" || cookie.Value.In != "cookie" || cookie.Value.Name != "session_id" {
		t.Fatalf("cookieAuth scheme = %+v, want apiKey in cookie named session_id", cookie.Value)
	}

	bearer := testDoc.Components.SecuritySchemes[securitySchemeBearer]
	if bearer == nil || bearer.Value == nil || bearer.Value.Type != "http" || bearer.Value.Scheme != "bearer" {
		t.Fatal("bearerAuth scheme missing or malformed")
	}

	if len(testDoc.Security) != 2 {
		t.Fatalf("doc.Security = %+v, want cookie or bearer requirement", testDoc.Security)
	}
}

func TestMarkPublic(t *testing.T) {
	op := &openapi3.Operation{}
	markPublic(op)
	if op.Security == nil || len(*op.Security) != 0 {
		t.Fatalf("op.Security = %+v, want an empty override", op.Security)
	}

	// A nil operation must not panic.
	markPublic(nil)
}

func TestSetMarksPublicRouteSecurity(t *testing.T) {
	Set[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel]("/api/test-public-route", false, consts.List)

	item := doc.Paths.Value("/api/test-public-route")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/test-public-route missing from document")
	}
	if item.Get.Security == nil || len(*item.Get.Security) != 0 {
		t.Fatalf("public op security = %+v, want an empty override", item.Get.Security)
	}
}

func TestSetKeepsAuthRouteSecurityDefault(t *testing.T) {
	Set[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel]("/api/test-auth-route", true, consts.List)

	item := doc.Paths.Value("/api/test-auth-route")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/test-auth-route missing from document")
	}
	if item.Get.Security != nil {
		t.Fatalf("auth op security = %+v, want nil so the document-level security applies", item.Get.Security)
	}
}
