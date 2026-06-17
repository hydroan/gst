package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httputil"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/go-querystring/query"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"golang.org/x/time/rate"
)

type action int

const (
	create action = iota
	delete_
	update
	patch
	list
	get
	create_many //nolint:staticcheck
	delete_many //nolint:staticcheck
	update_many //nolint:staticcheck
	patch_many  //nolint:staticcheck
)

var (
	ErrNotStringSlice        = errors.New("payload must be a string slice")
	ErrNotStructSlice        = errors.New("payload must be a struct slice")
	ErrUnsupportedHTTPMethod = errors.New("unsupported http method")
)

type Client struct {
	addr       string
	httpClient *http.Client
	username   string
	password   string
	token      string

	header      http.Header
	query       *model.Base
	queryRaw    string
	param       string
	apiPath     string
	debug       bool
	maxRetries  int
	retryWait   time.Duration
	rateLimiter *rate.Limiter

	ctx context.Context

	types.Logger
}

type Resp struct {
	Code      int             `json:"code,omitempty"`
	Msg       string          `json:"msg,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}
type batchReq struct {
	// IDs is the id list that should be batch delete.
	IDs any `json:"ids,omitempty"`
	// Items is the resource list that should be batch create/update/partial update.
	Items any `json:"items,omitempty"`
}

// New creates a new client instance with given base URL and options.
// The base URL must start with "http://" or "https://".
func New(addr string, opts ...Option) (*Client, error) {
	client := &Client{
		httpClient: http.DefaultClient,
		header:     http.Header{},
		addr:       strings.TrimRight(addr, "/"),
		ctx:        context.Background(),
		Logger:     zap.New(""),
	}
	client.header.Set("User-Agent", consts.FrameworkName)
	client.header.Set("Content-Type", "application/json")

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(client)
	}

	return client, nil
}

// QueryString build the query string from structured query parameters
// and raw query string.
func (c *Client) QueryString() (string, error) {
	if c.query == nil && len(c.queryRaw) == 0 {
		return "", nil
	}
	if c.query == nil {
		return c.queryRaw, nil
	}

	val, err := query.Values(c.query)
	if err != nil {
		return "", err
	}

	encoded := val.Encode()
	if len(encoded) == 0 {
		return c.queryRaw, nil
	}
	if len(c.queryRaw) != 0 {
		return c.queryRaw + "&" + encoded, nil
	}

	return encoded, nil
}

// RequestURL constructs the full request URL including base URL and query parameters.
func (c *Client) RequestURL() (string, error) {
	if !strings.HasPrefix(c.addr, "http://") && !strings.HasPrefix(c.addr, "https://") {
		return "", errors.New("addr must start with http:// or https://")
	}
	url := c.addr
	if len(c.apiPath) > 0 {
		url = fmt.Sprintf("%s/%s", c.addr, c.apiPath)
	}
	query, err := c.QueryString()
	if err != nil {
		return "", err
	}
	if len(query) > 0 {
		return fmt.Sprintf("%s?%s", url, query), nil
	}
	return url, nil
}

// Create send a POST request to create a new resource.
// payload can be []byte or struct/pointer that can be marshaled to JSON.
func (c *Client) Create(payload any) (*Resp, error) {
	return c.request(create, payload)
}

// Delete send a DELETE request to delete a resource.
func (c *Client) Delete(id string) (*Resp, error) {
	if len(id) == 0 {
		return nil, errors.New("id is required")
	}
	c.param = id
	return c.request(delete_, nil)
}

// Update send a PUT request to fully update a resource.
func (c *Client) Update(id string, payload any) (*Resp, error) {
	if len(id) == 0 {
		return nil, errors.New("id is required")
	}
	c.param = id
	return c.request(update, payload)
}

// Patch send a PATCH request to partially update a resource.
func (c *Client) Patch(id string, payload any) (*Resp, error) {
	if len(id) == 0 {
		return nil, errors.New("id is required")
	}
	c.param = id
	return c.request(patch, payload)
}

// List send a GET request to retrieve a list of resources.
// items must be a pointer to slice where items will be unmarshaled into.
// total will be set to the total number of items available.
func (c *Client) List(items any, total *int64) (*Resp, error) {
	if items == nil {
		return nil, errors.New("items cannot be nil")
	}
	if total == nil {
		return nil, errors.New("total cannot be nil")
	}

	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Pointer {
		return nil, errors.New("items must be a pointer to slice")
	}
	if val.Elem().Kind() != reflect.Slice {
		return nil, errors.New("items must be a pointer to slice")
	}
	resp, err := c.request(list, nil)
	if err != nil {
		return nil, err
	}
	responseList := new(struct {
		Items json.RawMessage `json:"items"`
		Total int64           `json:"total"`
	})
	if err := json.Unmarshal(resp.Data, responseList); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	if err := json.Unmarshal(responseList.Items, items); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	*total = responseList.Total
	return resp, nil
}

// Get send a GET request to get one resource by given id.
// The id parameter specifies which resource to retrieve.
// The dst parameter must be a pointer to struct where the resource will be unmarshaled into.
func (c *Client) Get(id string, dst any) (*Resp, error) {
	if len(id) == 0 {
		return nil, errors.New("id is required")
	}
	val := reflect.ValueOf(dst)
	if val.Kind() != reflect.Pointer {
		return nil, errors.New("dst must be a pointer to struct")
	}
	if val.Elem().Kind() != reflect.Struct {
		return nil, errors.New("dst must be a pointer to struct")
	}
	if !val.Elem().IsZero() {
		newVal := reflect.New(reflect.TypeOf(dst).Elem())
		val.Elem().Set(newVal.Elem())
	}
	c.param = id
	resp, err := c.request(get, nil)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resp.Data, dst); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	return resp, nil
}

func isStructSlice(payload any) bool {
	typ := reflect.TypeOf(payload)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Slice {
		return false
	}
	elemTyp := typ.Elem()
	for elemTyp.Kind() == reflect.Pointer {
		elemTyp = elemTyp.Elem()
	}
	return elemTyp.Kind() == reflect.Struct
}

func isStringSlice(payload any) bool {
	typ := reflect.TypeOf(payload)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.String
}

// CreateMany send a POST request to batch create multiple resources.
// payload should be a struct slice, eg: []User or []*User
func (c *Client) CreateMany(payload any) (*Resp, error) {
	if !isStructSlice(payload) {
		return nil, ErrNotStructSlice
	}
	return c.request(create_many, batchReq{Items: payload})
}

// DeleteMany send a DELETE request to batch delete multiple resources.
// payload should be a string slice contains id list.
func (c *Client) DeleteMany(payload any) (*Resp, error) {
	if !isStringSlice(payload) {
		return nil, ErrNotStringSlice
	}
	return c.request(delete_many, batchReq{IDs: payload})
}

// UpdateMany send a PUT request to batch update multiple resources.
// payload should be a struct slice, eg: []User or []*User
func (c *Client) UpdateMany(payload any) (*Resp, error) {
	if !isStructSlice(payload) {
		return nil, ErrNotStructSlice
	}
	return c.request(update_many, batchReq{Items: payload})
}

// PatchMany send a PATCH request to batch partially update multiple resources.
// payload should be a struct slice, eg: []User or []*User
func (c *Client) PatchMany(payload any) (*Resp, error) {
	if !isStructSlice(payload) {
		return nil, ErrNotStructSlice
	}
	return c.request(patch_many, batchReq{Items: payload})
}

// Request sends a single HTTP request using the given method and optional payload.
// It maps standard HTTP methods to resource actions:
//   - GET: list (base URL, with optional query from WithQuery/WithQueryPagination etc.)
//   - POST: create (base URL, body = payload)
//   - PUT: update (base URL + "/" + resource id; requires c.param to be set, e.g. by a prior Get/Update/Patch/Delete call)
//   - PATCH: patch (same URL as PUT)
//   - DELETE: delete (same URL as PUT)
//
// For GET and POST, the request URL is the client's base address (and apiPath if set).
// For PUT, PATCH, DELETE the URL includes the resource id from c.param; param is not set by Request.
// Prefer Get(id, dst), List(...), Create(payload), Update(id, payload), Patch(id, payload), Delete(id)
// when operating on resources by id; use Request only for list-like GET or create-like POST endpoints
// that do not need an id in the path (e.g. /api/version).
func (c *Client) Request(method string, payload any) (*Resp, error) {
	switch method {
	case http.MethodGet:
		return c.request(list, payload)
	case http.MethodPost:
		return c.request(create, payload)
	case http.MethodPut:
		return c.request(update, payload)
	case http.MethodPatch:
		return c.request(patch, payload)
	case http.MethodDelete:
		return c.request(delete_, payload)
	default:
		return nil, ErrUnsupportedHTTPMethod
	}
}

// request send a request to backend server.
// action determines the type of request,
// payload can be []byte or struct/pointer that can be marshaled to JSON.
func (c *Client) request(action action, payload any) (*Resp, error) {
	if c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(c.ctx); err != nil {
			return nil, errors.Wrap(err, "rate limit exceeded")
		}
	}

	var url string
	var err error
	var method string
	baseURL := c.addr
	if len(c.apiPath) > 0 {
		baseURL = fmt.Sprintf("%s/%s", c.addr, c.apiPath)
	}
	switch action {
	case create:
		method = http.MethodPost
		url = baseURL
	case delete_:
		method = http.MethodDelete
		url = fmt.Sprintf("%s/%s", baseURL, c.param)
	case update:
		method = http.MethodPut
		url = fmt.Sprintf("%s/%s", baseURL, c.param)
	case patch:
		method = http.MethodPatch
		url = fmt.Sprintf("%s/%s", baseURL, c.param)
	case create_many:
		method = http.MethodPost
		url = baseURL + "/batch"
	case delete_many:
		method = http.MethodDelete
		url = baseURL + "/batch"
	case update_many:
		method = http.MethodPut
		url = baseURL + "/batch"
	case patch_many:
		method = http.MethodPatch
		url = baseURL + "/batch"
	case list:
		method = http.MethodGet
		url, err = c.RequestURL()
	case get:
		method = http.MethodGet
		url = fmt.Sprintf("%s/%s", baseURL, c.param)
	}
	if err != nil {
		return nil, errors.Wrap(err, "invalid request url")
	}

	var reader io.Reader
	if payload != nil {
		switch v := payload.(type) {
		case []byte:
			reader = bytes.NewReader(v)
		default:
			var data []byte
			if data, err = json.Marshal(v); err != nil {
				return nil, errors.Wrap(err, "failed to marshal payload")
			}
			reader = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	if c.ctx != nil {
		req = req.WithContext(c.ctx)
	}
	if len(c.username) > 0 {
		req.SetBasicAuth(c.username, c.password)
	}
	if len(c.token) > 0 {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	maps.Copy(req.Header, c.header)

	if c.debug {
		dump, _ := httputil.DumpRequest(req, true)
		fmt.Println(string(dump))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request")
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, errors.Wrap(err, "failed to copy response body")
	}
	if c.debug {
		dump, _ := httputil.DumpResponse(resp, true)
		fmt.Println(string(dump))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("response status code: %d, body: %s", resp.StatusCode, buf.String())
	}

	if len(buf.Bytes()) != 0 {
		res := new(Resp)
		if err := json.Unmarshal(buf.Bytes(), res); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal response: "+buf.String())
		}
		if res.Code != 0 {
			return nil, fmt.Errorf("response status code: %d, code: %d, msg: %s, body: %s", resp.StatusCode, res.Code, res.Msg, buf.String())
		}
		return res, nil
	}

	// Delete or BatchDelete response is empty with http status 204.
	return &Resp{}, nil
}

// StreamCallback is a function type for handling stream events.
// It receives an SSE event and returns an error if processing should stop.
type StreamCallback func(event types.Event) error

// Stream sends a POST request and processes the response as a Server-Sent Events (SSE) stream.
// The callback function will be called for each event received, allowing immediate processing.
//
// Parameters:
//   - payload: The request payload (can be []byte or struct/pointer that can be marshaled to JSON)
//   - callback: Function to handle each SSE event. Return an error to stop streaming.
//
// Example:
//
//	err := client.Stream(payload, func(event types.Event) error {
//	    fmt.Printf("Event: %s, Data: %v\n", event.Event, event.Data)
//	    return nil // Continue streaming
//	})
func (c *Client) Stream(payload any, callback StreamCallback) error {
	return c.StreamURL(http.MethodPost, "", payload, callback)
}

// parseSSEStream parses a Server-Sent Events stream and calls the callback for each event.
func (c *Client) parseSSEStream(body io.Reader, callback StreamCallback) error {
	scanner := bufio.NewScanner(body)
	event := types.Event{}
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line indicates end of event
		if line == "" {
			if len(dataLines) > 0 {
				// Join all data lines with newline (SSE spec allows multiple data fields)
				event.Data = strings.Join(dataLines, "\n")
				if err := callback(event); err != nil {
					return err
				}
				// Reset for next event
				event = types.Event{}
				dataLines = nil
			}
			continue
		}

		// Parse SSE field
		if strings.HasPrefix(line, "id:") {
			event.ID = strings.TrimSpace(line[3:])
		} else if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "retry:") {
			retryStr := strings.TrimSpace(line[6:])
			if retry, err := parseInt(retryStr); err == nil {
				event.Retry = retry
			}
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(line[5:])
			// Check for [DONE] marker
			if data == "[DONE]" {
				return nil
			}
			dataLines = append(dataLines, data)
		}
		// Ignore comments (lines starting with :)
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to read stream")
	}

	// Handle last event if stream ends without empty line
	if len(dataLines) > 0 {
		event.Data = strings.Join(dataLines, "\n")
		if err := callback(event); err != nil {
			return err
		}
	}

	return nil
}

// StreamPrint sends a POST request and prints each SSE event as it arrives.
// This is a convenience method that automatically prints events to stdout.
// JSON data will be pretty-printed if possible.
//
// Parameters:
//   - payload: The request payload (can be []byte or struct/pointer that can be marshaled to JSON)
//
// Example:
//
//	err := client.StreamPrint(payload)
func (c *Client) StreamPrint(payload any) error {
	return c.Stream(payload, c.defaultPrintCallback)
}

// StreamURL sends a request to a custom URL and processes the response as a Server-Sent Events (SSE) stream.
// This allows streaming from any endpoint, not just the base address.
// If url is empty, it uses the base address.
//
// Parameters:
//   - method: HTTP method (e.g., "GET", "POST")
//   - url: Full URL or path relative to base address (empty string uses base address)
//   - payload: The request payload (can be []byte or struct/pointer that can be marshaled to JSON, or nil)
//   - callback: Function to handle each SSE event. Return an error to stop streaming.
//
// Example:
//
//	err := client.StreamURL("POST", "/api/chat/stream", payload, func(event types.Event) error {
//	    fmt.Printf("Event: %s, Data: %v\n", event.Event, event.Data)
//	    return nil
//	})
func (c *Client) StreamURL(method, url string, payload any, callback StreamCallback) error {
	if callback == nil {
		return errors.New("callback cannot be nil")
	}

	if c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(c.ctx); err != nil {
			return errors.Wrap(err, "rate limit exceeded")
		}
	}

	// Build full URL
	var fullURL string
	if url == "" {
		fullURL = c.addr
	} else if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		fullURL = url
	} else {
		fullURL = c.addr + "/" + strings.TrimLeft(url, "/")
	}

	var reader io.Reader
	if payload != nil {
		switch v := payload.(type) {
		case []byte:
			reader = bytes.NewReader(v)
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return errors.Wrap(err, "failed to marshal payload")
			}
			reader = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	if c.ctx != nil {
		req = req.WithContext(c.ctx)
	}
	if len(c.username) > 0 {
		req.SetBasicAuth(c.username, c.password)
	}
	if len(c.token) > 0 {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	maps.Copy(req.Header, c.header)
	// Set Accept header for SSE
	req.Header.Set("Accept", "text/event-stream")

	if c.debug {
		dump, _ := httputil.DumpRequest(req, true)
		fmt.Println(string(dump))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to request")
	}
	defer resp.Body.Close()

	if c.debug {
		dump, _ := httputil.DumpResponse(resp, false)
		fmt.Println(string(dump))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		_, _ = io.Copy(buf, resp.Body)
		return fmt.Errorf("response status code: %d, body: %s", resp.StatusCode, buf.String())
	}

	// Check if response is SSE format
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		// If not SSE, read as regular response
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, resp.Body); err != nil {
			return errors.Wrap(err, "failed to copy response body")
		}
		if len(buf.Bytes()) != 0 {
			res := new(Resp)
			if err := json.Unmarshal(buf.Bytes(), res); err != nil {
				return errors.Wrap(err, "failed to unmarshal response: "+buf.String())
			}
			if res.Code != 0 {
				return fmt.Errorf("response status code: %d, code: %d, msg: %s, body: %s", resp.StatusCode, res.Code, res.Msg, buf.String())
			}
		}
		return nil
	}

	// Parse SSE stream
	return c.parseSSEStream(resp.Body, callback)
}

// StreamPrintURL sends a request to a custom URL and prints each SSE event as it arrives.
// This is a convenience method that automatically prints events to stdout.
//
// Parameters:
//   - method: HTTP method (e.g., "GET", "POST")
//   - url: Full URL or path relative to base address (empty string uses base address)
//   - payload: The request payload (can be []byte or struct/pointer that can be marshaled to JSON, or nil)
//
// Example:
//
//	err := client.StreamPrintURL("POST", "/api/chat/stream", payload)
func (c *Client) StreamPrintURL(method, url string, payload any) error {
	return c.StreamURL(method, url, payload, c.defaultPrintCallback)
}

// defaultPrintCallback is the default callback used by StreamPrint and StreamPrintURL.
func (c *Client) defaultPrintCallback(event types.Event) error {
	// Print event immediately
	if event.Event != "" {
		fmt.Printf("[%s] ", event.Event)
	}
	if event.ID != "" {
		fmt.Printf("(id: %s) ", event.ID)
	}

	// Try to pretty-print JSON data
	dataStr := fmt.Sprintf("%v", event.Data)
	if strings.TrimSpace(dataStr) != "" {
		var jsonData any
		if err := json.Unmarshal([]byte(dataStr), &jsonData); err == nil {
			// It's valid JSON, pretty print it
			prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
			if err == nil {
				fmt.Println(string(prettyJSON))
			} else {
				fmt.Println(dataStr)
			}
		} else {
			// Not JSON, print as-is
			fmt.Println(dataStr)
		}
	} else {
		fmt.Println()
	}
	return nil
}

// parseInt parses an integer string and returns the value.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
