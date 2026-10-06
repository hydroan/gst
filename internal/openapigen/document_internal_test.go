package openapigen

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/apidoc"
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

// TestSetDocSecurityDeclaresTheRegisteredSchemes pins that the document
// declares the schemes the project's middleware registered and no other: a
// project registering none declares none and requires nothing, and once a
// cookie and a bearer scheme are registered both are declared, each a
// requirement of its own, in name order.
func TestSetDocSecurityDeclaresTheRegisteredSchemes(t *testing.T) {
	bare := &openapi3.T{Components: &openapi3.Components{}}
	setDocSecurity(bare)
	if len(bare.Components.SecuritySchemes) != 0 || bare.Security != nil {
		t.Fatalf("with no scheme registered the document declares %+v and requires %+v, want nothing", bare.Components.SecuritySchemes, bare.Security)
	}

	apidoc.RegisterSecurityScheme("sampleCookie", apidoc.SecurityScheme{Type: "apiKey", In: "cookie", Name: "sample_session", Description: "the sample session cookie"})
	apidoc.RegisterSecurityScheme("sampleBearer", apidoc.SecurityScheme{Type: "http", Scheme: "bearer"})
	testDoc := &openapi3.T{Components: &openapi3.Components{}}
	setDocSecurity(testDoc)

	cookie := testDoc.Components.SecuritySchemes["sampleCookie"]
	if cookie == nil || cookie.Value == nil {
		t.Fatal("sampleCookie scheme missing")
	}
	if cookie.Value.Type != "apiKey" || cookie.Value.In != "cookie" || cookie.Value.Name != "sample_session" || cookie.Value.Description != "the sample session cookie" {
		t.Fatalf("sampleCookie scheme = %+v, want apiKey in cookie named sample_session", cookie.Value)
	}
	bearer := testDoc.Components.SecuritySchemes["sampleBearer"]
	if bearer == nil || bearer.Value == nil || bearer.Value.Type != "http" || bearer.Value.Scheme != "bearer" {
		t.Fatal("sampleBearer scheme missing or malformed")
	}
	want := openapi3.SecurityRequirements{{"sampleBearer": []string{}}, {"sampleCookie": []string{}}}
	if !reflect.DeepEqual(testDoc.Security, want) {
		t.Fatalf("doc.Security = %+v, want one requirement per scheme in name order", testDoc.Security)
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
