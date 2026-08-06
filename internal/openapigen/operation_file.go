package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_IMPORT)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_IMPORT)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The upload is documented inline as multipart/form-data rather than as a
	// JSON request component, so no request schema is registered under reqKey.
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(path, consts.Import),
		Summary:     summary(path, consts.Import, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Import, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Import, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: importFileRequestBody(),
		Responses:   newResponses[RSP](rspKey),
	}
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
// filters plus the file-format selector and a binary file response.
func setExport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.Export),
		Summary:     summary(path, consts.Export, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Export, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Export, typ),
		Parameters:  append(parseParametersFromPath(path), exportFormatParameter()),
		Responses:   exportFileResponses(),
	}
	addQueryParameters[M, REQ, RSP](pathItem.Get)
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

// exportFileResponses documents the export download: a 200 response whose body
// is the generated csv or xlsx file delivered as a binary stream.
func exportFileResponses() *openapi3.Responses {
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
	return openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: response}))
}
