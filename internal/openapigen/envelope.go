package openapigen

import "github.com/hydroan/gst/internal/types"

// The Go shapes the documented request and response bodies are generated from:
// the payload wrapper of a batch request, whose items the flow requires, the
// success envelope the controllers answer with in its single-record, list
// and batch forms, and the data of a failure naming the fields the request
// was refused on.

type apiBatchRequest[T any] struct {
	Items []T `json:"items" binding:"required"`
}

type apiResponse[T any] struct {
	Data    T      `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

type apiListResponse[T any] struct {
	Data    listData[T] `json:"data"`
	Msg     string      `json:"msg"`
	TraceID string      `json:"trace_id"`
}

type listData[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type apiBatchResponse[T any] struct {
	Data    batchData[T] `json:"data"`
	Msg     string       `json:"msg"`
	TraceID string       `json:"trace_id"`
}

type batchData[T any] struct {
	Items []T `json:"items"`
}

type failureData struct {
	FieldViolations []types.FieldViolation `json:"field_violations"`
}
