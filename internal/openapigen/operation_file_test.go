package openapigen

import (
	"reflect"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types/consts"
)

type openapiExportModel struct {
	// Name is the sample name.
	Name string `json:"name" query:"name"`

	modelregistry.Base
}

func TestSetExportDocumentsFileDownload(t *testing.T) {
	pathItem := &openapi3.PathItem{}
	setExport[*openapiExportModel, *openapiExportModel, *openapiExportModel]("/api/exports/export", pathItem)

	op := pathItem.Get
	if op == nil {
		t.Fatal("setExport did not document a GET operation")
	}
	if op.OperationID == "" || op.Summary == "" {
		t.Fatalf("export operation id/summary missing: id=%q summary=%q", op.OperationID, op.Summary)
	}
	if len(op.Tags) == 0 || op.Tags[0] != "exports" {
		t.Fatalf("export tags = %v, want [exports]", op.Tags)
	}

	// The 200 response advertises binary file downloads instead of a JSON envelope.
	if op.Responses == nil || op.Responses.Value("200") == nil || op.Responses.Value("200").Value == nil {
		t.Fatalf("export responses = %v, want an inline status 200", op.Responses)
	}
	content := op.Responses.Value("200").Value.Content
	for _, mediaType := range []string{"text/csv", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"} {
		mt := content[mediaType]
		if mt == nil || mt.Schema == nil || mt.Schema.Value == nil || mt.Schema.Value.Format != "binary" {
			t.Fatalf("export 200 media type %q is not a binary file schema: %#v", mediaType, mt)
		}
	}

	// The file-format query parameter documents the supported formats.
	params := queryParametersByName(t, op)
	format := params[consts.QUERY_FORMAT]
	if format == nil || format.Schema == nil || format.Schema.Value == nil {
		t.Fatalf("export query parameters = %v, want %q", parameterNames(params), consts.QUERY_FORMAT)
	}
	if got := format.Schema.Value.Enum; !reflect.DeepEqual(got, []any{"csv", "xlsx"}) {
		t.Fatalf("export %q enum = %v, want [csv xlsx]", consts.QUERY_FORMAT, got)
	}
	// Export shares the list filters, so model field queries are documented too.
	if params["name"] == nil {
		t.Fatalf("export query parameters = %v, want model filter %q", parameterNames(params), "name")
	}
}

type openapiImportModel struct {
	// Name is the sample name.
	Name string `json:"name"`

	modelregistry.Base
}

func TestSetImportDocumentsMultipartUpload(t *testing.T) {
	pathItem := &openapi3.PathItem{}
	setImport[*openapiImportModel, *openapiImportModel, *openapiImportModel]("/api/imports/import", pathItem)

	op := pathItem.Post
	if op == nil {
		t.Fatal("setImport did not document a POST operation")
	}
	if op.OperationID == "" || op.Summary == "" {
		t.Fatalf("import operation id/summary missing: id=%q summary=%q", op.OperationID, op.Summary)
	}
	if len(op.Tags) == 0 || op.Tags[0] != "imports" {
		t.Fatalf("import tags = %v, want [imports]", op.Tags)
	}

	// The request body is a required multipart/form-data upload with a binary "file" field.
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		t.Fatal("import operation is missing a request body")
	}
	body := op.RequestBody.Value
	if !body.Required {
		t.Fatal("import request body should be required")
	}
	mediaType := body.Content["multipart/form-data"]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		t.Fatalf("import request body content = %v, want multipart/form-data", body.Content)
	}
	fileSchema := mediaType.Schema.Value.Properties["file"]
	if fileSchema == nil || fileSchema.Value == nil || fileSchema.Value.Format != "binary" {
		t.Fatalf("import request body file property = %#v, want a binary schema", fileSchema)
	}
	if !slices.Contains(mediaType.Schema.Value.Required, "file") {
		t.Fatalf("import request body required = %v, want to include \"file\"", mediaType.Schema.Value.Required)
	}

	// The success envelope is documented as a 200 response.
	if op.Responses == nil || op.Responses.Value("200") == nil {
		t.Fatalf("import responses = %v, want status 200", op.Responses)
	}
}
