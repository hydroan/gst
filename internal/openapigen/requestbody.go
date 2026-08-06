package openapigen

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// baseAutoFieldNames are the JSON names of the fields the framework fills in
// itself, so a request body never asks the caller to supply them.
var baseAutoFieldNames = map[string]bool{
	"id":         true,
	"created_at": true,
	"created_by": true,
	"updated_at": true,
	"updated_by": true,
	"deleted_at": true,
	"deleted_by": true,
}

// removeBaseAutoFieldsFromRequestBody hides the framework-managed fields in the
// request body of a single-record CRUD operation.
func removeBaseAutoFieldsFromRequestBody(op *openapi3.Operation) {
	requestBody := resolveRequestBody(op)
	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// hold the write lock while the schema is modified
	docMutex.Lock()
	defer docMutex.Unlock()

	for _, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}
		removeBaseAutoFields(mediaType.Schema.Value)
	}
}

// removeBaseAutoFieldsFromBatchRequestBody hides the framework-managed fields
// in the request body of a batch CRUD operation, where the payload sits in an
// items array instead of at the top level.
func removeBaseAutoFieldsFromBatchRequestBody(op *openapi3.Operation) {
	requestBody := resolveRequestBody(op)
	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// hold the write lock while the schema is modified
	docMutex.Lock()
	defer docMutex.Unlock()

	for _, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}
		schema := mediaType.Schema.Value

		if itemSchema := batchItemSchema(schema); itemSchema != nil {
			removeBaseAutoFields(itemSchema)
		}
		removeBaseAutoFieldsFromBatchExample(schema)
	}
}

// resolveRequestBody returns the request body an operation documents, following
// the component reference when the operation carries one. It returns nil when
// the operation declares no body or the referenced component is missing.
func resolveRequestBody(op *openapi3.Operation) *openapi3.RequestBody {
	if op == nil || op.RequestBody == nil {
		return nil
	}
	if op.RequestBody.Ref == "" {
		return op.RequestBody.Value
	}

	docMutex.RLock()
	defer docMutex.RUnlock()
	if doc.Components.RequestBodies == nil {
		return nil
	}
	refKey := strings.TrimPrefix(op.RequestBody.Ref, "#/components/requestBodies/")
	if rb, exists := doc.Components.RequestBodies[refKey]; exists && rb.Value != nil {
		return rb.Value
	}
	return nil
}

// removeBaseAutoFields drops the framework-managed fields from one object
// schema: its properties, its required list and its example.
func removeBaseAutoFields(schema *openapi3.Schema) {
	if schema.Properties != nil {
		for field := range baseAutoFieldNames {
			delete(schema.Properties, field)
		}
	}

	if len(schema.Required) > 0 {
		newRequired := []string{}
		for _, req := range schema.Required {
			if !baseAutoFieldNames[req] {
				newRequired = append(newRequired, req)
			}
		}
		schema.Required = newRequired
	}

	if schema.Example != nil {
		if exampleMap, ok := schema.Example.(map[string]any); ok {
			for field := range baseAutoFieldNames {
				delete(exampleMap, field)
			}
		}
	}
}

// batchItemSchema returns the element schema of the items array a batch request
// body carries, or nil when the body does not describe one.
func batchItemSchema(schema *openapi3.Schema) *openapi3.Schema {
	if schema.Properties == nil {
		return nil
	}
	itemsProp, exists := schema.Properties["items"]
	if !exists || itemsProp.Value == nil || itemsProp.Value.Items == nil {
		return nil
	}
	return itemsProp.Value.Items.Value
}

// removeBaseAutoFieldsFromBatchExample drops the framework-managed fields from
// every element of the example of the whole batch request.
func removeBaseAutoFieldsFromBatchExample(schema *openapi3.Schema) {
	if schema.Example == nil {
		return
	}
	exampleMap, ok := schema.Example.(map[string]any)
	if !ok {
		return
	}
	items, exists := exampleMap["items"]
	if !exists {
		return
	}

	switch itemsArray := items.(type) {
	case []map[string]any:
		for _, item := range itemsArray {
			for field := range baseAutoFieldNames {
				delete(item, field)
			}
		}
	case []any:
		for _, item := range itemsArray {
			if itemMap, ok := item.(map[string]any); ok {
				for field := range baseAutoFieldNames {
					delete(itemMap, field)
				}
			}
		}
	}
}
