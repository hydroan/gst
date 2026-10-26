package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
)

// Import and export exchange whole record sets as files rather than as JSON
// bodies, so both operations are documented apart from the CRUD ones.

// Media types and file-format query values documented for the import/export
// operations. They mirror the values produced by the import/export controllers:
// export streams a csv or xlsx file, and import reads a multipart file upload.
const (
	exportFormatCSV  = "csv"
	exportFormatXLSX = "xlsx"

	exportMediaTypeCSV  = "text/csv"
	exportMediaTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	// importFileField is the multipart form field name read by the import
	// controller (c.FormFile("file")); the request body schema must match it.
	importFileField = "file"
)

// setImport documents the import action (POST /{path}/import). The controller
// reads a single uploaded file, so the request body is a multipart/form-data
// upload with a required binary "file" field, and the response reuses the
// standard success envelope returned by the controller.
func setImport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Import)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Import)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The upload is documented inline as multipart/form-data rather than as a
	// JSON request component, so no request schema is registered under reqKey.
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, consts.Import, typ, custom)
	op.RequestBody = importFileRequestBody()
	op.Responses = newResponses(rspKey, consts.Import, custom)
	pathItem.Post = op
}

// importFileRequestBody documents the multipart/form-data upload consumed by
// the import controller: a single required binary file field.
func importFileRequestBody() *openapi3.RequestBodyRef {
	return &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Description: "The file to import.",
			Required:    true,
			Content: openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeObject},
							Properties: openapi3.Schemas{
								importFileField: {
									Value: &openapi3.Schema{
										Type:   &openapi3.Types{openapi3.TypeString},
										Format: "binary",
									},
								},
							},
							Required: []string{importFileField},
						},
					},
				},
			},
		},
	}
}

// setExport documents the export action (GET /{path}/export). The controller
// filters resources with the same query parameters as the list action and
// streams the result as a downloadable file, so the operation carries the list
// filters plus the file-format selector and a binary file response, and the
// cap on the records listed where the controller lists them: a virtual
// resource's export service lists its records itself.
func setExport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()

	op := newOperation(path, consts.Export, reflect.TypeFor[M](), custom)
	op.Parameters = append(op.Parameters, exportFormatParameter())
	if !modelregistry.IsVirtual(*new(M)) {
		op.Parameters = append(op.Parameters, exportLimitParameter())
	}
	op.Responses = exportFileResponses(custom)
	pathItem.Get = op
	addQueryParameters[M, REQ, RSP](pathItem.Get, false)
}

// exportFormatParameter documents the file-format query parameter accepted by
// the export controller, restricting the value to the supported formats.
func exportFormatParameter() *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        consts.QUERY_FORMAT,
			In:          "query",
			Required:    false,
			Description: "The export file format.",
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeString},
					Enum: []any{exportFormatCSV, exportFormatXLSX},
				},
			},
		},
	}
}

// exportLimitParameter documents the cap on the records the export controller
// lists for a table-backed resource: a positive integer caps them, none or
// zero or less lists every record the query matches.
func exportLimitParameter() *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        consts.QUERY_LIMIT,
			In:          "query",
			Required:    false,
			Description: "The most records to export; none, zero or less exports every record the query matches.",
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}},
			},
		},
	}
}

// exportFileResponses documents the export download: a 200 response whose body
// is the generated csv or xlsx file delivered as a binary stream, and the
// failures the export answers (see responsesOf), custom when the action's
// types are the project's own.
func exportFileResponses(custom bool) *openapi3.Responses {
	fileSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:   &openapi3.Types{openapi3.TypeString},
			Format: "binary",
		},
	}
	response := openapi3.NewResponse().
		WithDescription("The exported file.").
		WithContent(openapi3.Content{
			exportMediaTypeCSV:  openapi3.NewMediaType().WithSchemaRef(fileSchema),
			exportMediaTypeXLSX: openapi3.NewMediaType().WithSchemaRef(fileSchema),
		})
	return responsesOf(&openapi3.ResponseRef{Value: response}, consts.Export, custom)
}
