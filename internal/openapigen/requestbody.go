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

// removeFieldsFromRequestBody removes the given fields from the RequestBody of a single CRUD operation.
func removeFieldsFromRequestBody(op *openapi3.Operation, fieldsToRemove ...string) {
	if op == nil || op.RequestBody == nil {
		return
	}

	// build a map for easy lookup
	removeMap := make(map[string]bool)
	for _, field := range fieldsToRemove {
		removeMap[field] = true
	}

	// fall back to the default fields when the caller passes none
	if len(fieldsToRemove) == 0 {
		removeMap = baseAutoFieldNames
	}

	// resolve the RequestBodyRef
	var requestBody *openapi3.RequestBody

	if op.RequestBody.Ref != "" {
		// a reference: look the actual RequestBody up in components
		docMutex.RLock()
		if doc.Components.RequestBodies != nil {
			refKey := strings.TrimPrefix(op.RequestBody.Ref, "#/components/requestBodies/")
			if rb, exists := doc.Components.RequestBodies[refKey]; exists && rb.Value != nil {
				requestBody = rb.Value
			}
		}
		docMutex.RUnlock()
	} else if op.RequestBody.Value != nil {
		requestBody = op.RequestBody.Value
	}

	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// hold the write lock while the schema is modified
	docMutex.Lock()
	defer docMutex.Unlock()

	// handle each content type
	for contentType, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}

		schema := mediaType.Schema.Value

		// remove the fields from properties
		if schema.Properties != nil {
			for field := range removeMap {
				delete(schema.Properties, field)
			}
		}

		// remove the fields from required
		if len(schema.Required) > 0 {
			newRequired := []string{}
			for _, req := range schema.Required {
				if !removeMap[req] {
					newRequired = append(newRequired, req)
				}
			}
			schema.Required = newRequired
		}

		// handle the example
		if schema.Example != nil {
			if exampleMap, ok := schema.Example.(map[string]any); ok {
				for field := range removeMap {
					delete(exampleMap, field)
				}
			}
		}

		// write the content back
		requestBody.Content[contentType] = mediaType
	}
}

// removeFieldsFromBatchRequestBody removes the given fields from the RequestBody of a batch CRUD operation.
func removeFieldsFromBatchRequestBody(op *openapi3.Operation, fieldsToRemove ...string) {
	if op == nil || op.RequestBody == nil {
		return
	}

	// build a map for easy lookup
	removeMap := make(map[string]bool)
	for _, field := range fieldsToRemove {
		removeMap[field] = true
	}

	// fall back to the default fields when the caller passes none
	if len(fieldsToRemove) == 0 {
		removeMap = baseAutoFieldNames
	}

	// resolve the RequestBodyRef
	var requestBody *openapi3.RequestBody

	if op.RequestBody.Ref != "" {
		// a reference: look the actual RequestBody up in components
		docMutex.RLock()
		if doc.Components.RequestBodies != nil {
			refKey := strings.TrimPrefix(op.RequestBody.Ref, "#/components/requestBodies/")
			if rb, exists := doc.Components.RequestBodies[refKey]; exists && rb.Value != nil {
				requestBody = rb.Value
			}
		}
		docMutex.RUnlock()
	} else if op.RequestBody.Value != nil {
		requestBody = op.RequestBody.Value
	}

	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// hold the write lock while the schema is modified
	docMutex.Lock()
	defer docMutex.Unlock()

	// handle each content type
	for contentType, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}

		schema := mediaType.Schema.Value

		// a batch operation carries the payload in an items array
		if schema.Properties != nil {
			if itemsProp, exists := schema.Properties["items"]; exists {
				if itemsProp.Value != nil && itemsProp.Value.Items != nil && itemsProp.Value.Items.Value != nil {
					itemSchema := itemsProp.Value.Items.Value

					// remove the fields from every element of items
					if itemSchema.Properties != nil {
						for field := range removeMap {
							delete(itemSchema.Properties, field)
						}
					}

					// remove the fields from required
					if len(itemSchema.Required) > 0 {
						newRequired := []string{}
						for _, req := range itemSchema.Required {
							if !removeMap[req] {
								newRequired = append(newRequired, req)
							}
						}
						itemSchema.Required = newRequired
					}

					// handle the example of items
					if itemSchema.Example != nil {
						if exampleMap, ok := itemSchema.Example.(map[string]any); ok {
							for field := range removeMap {
								delete(exampleMap, field)
							}
						}
					}
				}
			}
		}

		// handle the example of the whole batch request
		if schema.Example != nil {
			if exampleMap, ok := schema.Example.(map[string]any); ok {
				if items, exists := exampleMap["items"]; exists {
					if itemsArray, ok := items.([]map[string]any); ok {
						for _, item := range itemsArray {
							for field := range removeMap {
								delete(item, field)
							}
						}
					} else if itemsArray, ok := items.([]any); ok {
						for i, item := range itemsArray {
							if itemMap, ok := item.(map[string]any); ok {
								for field := range removeMap {
									delete(itemMap, field)
								}
								itemsArray[i] = itemMap
							}
						}
					}
				}
			}
		}

		// write the content back
		requestBody.Content[contentType] = mediaType
	}
}
