package client

// ListResult is the response envelope of the framework's standard List action.
// Custom list responses with extra fields decode into their own RSP type
// instead; nothing forces this shape on them.
type ListResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// ItemsPayload is the request body of the standard batch create, update and
// patch routes.
type ItemsPayload[T any] struct {
	Items []T `json:"items"`
}

// IDsPayload is the request body of the standard batch delete route.
type IDsPayload struct {
	IDs []string `json:"ids"`
}

// BatchItems builds the items body for the standard /batch routes.
func BatchItems[T any](items []T) ItemsPayload[T] { return ItemsPayload[T]{Items: items} }

// BatchIDs builds the ids body for the standard batch delete route.
func BatchIDs(ids []string) IDsPayload { return IDsPayload{IDs: ids} }
