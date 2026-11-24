package apidoc_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
	internalapidoc "github.com/hydroan/gst/internal/apidoc"
	"github.com/stretchr/testify/require"
)

// TestTheRegistrationsReachTheFrameworksRegistry pins the public package as a
// forwarder: each type is the framework's own, and what a project registers
// through it is what the document generator looks up.
func TestTheRegistrationsReachTheFrameworksRegistry(t *testing.T) {
	require.Equal(t, reflect.TypeFor[internalapidoc.StructDoc](), reflect.TypeFor[apidoc.StructDoc]())
	require.Equal(t, reflect.TypeFor[internalapidoc.EnumValue](), reflect.TypeFor[apidoc.EnumValue]())
	require.Equal(t, reflect.TypeFor[internalapidoc.EnumDoc](), reflect.TypeFor[apidoc.EnumDoc]())
	require.Equal(t, reflect.TypeFor[internalapidoc.SecurityScheme](), reflect.TypeFor[apidoc.SecurityScheme]())
	require.Equal(t, reflect.TypeFor[internalapidoc.OperationDoc](), reflect.TypeFor[apidoc.OperationDoc]())

	apidoc.Register("sample/model", "ForwardedRecord", apidoc.StructDoc{
		Comment: "ForwardedRecord is a sample record.",
		Fields:  map[string]string{"Name": "Name is its name."},
	})
	doc, ok := internalapidoc.Lookup("sample/model", "ForwardedRecord")
	require.True(t, ok)
	require.Equal(t, "ForwardedRecord is a sample record.", doc.Comment)
	require.Equal(t, map[string]string{"Name": "Name is its name."}, doc.Fields)

	apidoc.RegisterEnum("sample/model", "ForwardedKind", apidoc.EnumDoc{
		Comment: "ForwardedKind is a sample kind.",
		Values:  []apidoc.EnumValue{{Value: "plain", Comment: "ForwardedKindPlain is the plain kind."}},
	})
	enum, ok := internalapidoc.LookupEnum("sample/model", "ForwardedKind")
	require.True(t, ok)
	require.Equal(t, []apidoc.EnumValue{{Value: "plain", Comment: "ForwardedKindPlain is the plain kind."}}, enum.Values)

	scheme := apidoc.SecurityScheme{Type: "http", Scheme: "bearer", Description: "A sample token."}
	apidoc.RegisterSecurityScheme("forwardedScheme", scheme)
	require.Equal(t, scheme, internalapidoc.SecuritySchemes()["forwardedScheme"])

	apidoc.RegisterOperation(http.MethodGet, "/api/forwarded-records/:id", apidoc.OperationDoc{Summary: "Read a forwarded record"})
	operation, ok := internalapidoc.LookupOperation(http.MethodGet, "/api/forwarded-records/{id}")
	require.True(t, ok)
	require.Equal(t, "Read a forwarded record", operation.Summary)
}
