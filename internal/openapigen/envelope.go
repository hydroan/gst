package openapigen

// The Go shapes the documented request and response bodies are generated from:
// the payload wrapper of a batch request, and the success envelope the
// controllers answer with in its single-record, list and batch forms.

type apiBatchRequest[T any] struct {
	Items []T `json:"items"`
}

type apiResponse[T any] struct {
	Code    int    `json:"code"`
	Data    T      `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

type apiListResponse[T any] struct {
	Code    int         `json:"code"`
	Data    listData[T] `json:"data"`
	Msg     string      `json:"msg"`
	TraceID string      `json:"trace_id"`
}

type listData[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type apiBatchResponse[T any] struct {
	Code    int          `json:"code"`
	Data    batchData[T] `json:"data"`
	Msg     string       `json:"msg"`
	TraceID string       `json:"trace_id"`
}

type batchData[T any] struct {
	Items   []T            `json:"items"`
	Options map[string]any `json:"options"`
	Summary batchSummary   `json:"summary"`
}

type batchSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}
