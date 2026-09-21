package openapigen

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/config"
)

// TestSetDocInfoAlwaysDeclaresAVersion asserts that the document declares a
// version even when the application never sets one. An unset app version is
// normal, while the spec requires info.version to be a non-empty string.
func TestSetDocInfoAlwaysDeclaresAVersion(t *testing.T) {
	config.App = new(config.Config)

	testDoc := &openapi3.T{Components: &openapi3.Components{}}
	setDocInfo(testDoc)

	if testDoc.Info == nil {
		t.Fatal("info is missing")
	}
	if testDoc.Info.Version == "" {
		t.Fatalf("info.version = %q, want a non-empty version", testDoc.Info.Version)
	}
}

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
