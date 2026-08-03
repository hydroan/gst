package client

import (
	"encoding/json"
	"net/http"
)

// Envelope is the standard response envelope a gst backend answers with. The
// verb functions decode its Data field into the business RSP type; Envelope
// itself is returned by Do for callers that need the envelope details, such
// as TraceID or the response cookies.
type Envelope struct {
	Code    int             `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	TraceID string          `json:"trace_id,omitempty"`
	Cookies []*http.Cookie  `json:"-"`
}

// Cookie returns the named response cookie, or nil when the response did not
// set it.
func (e *Envelope) Cookie(name string) *http.Cookie {
	if e == nil {
		return nil
	}
	for _, cookie := range e.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

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
