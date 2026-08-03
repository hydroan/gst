package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Client is a service-level HTTP client for one gst backend: it carries the
// base address, the connection with its cookie jar, credentials and shared
// headers. Per-request state (path, payload, query parameters) is passed per
// call, so one client serves every endpoint of the service and can be reused
// across requests safely.
type Client struct {
	addr       string
	httpClient *http.Client
	username   string
	password   string
	token      string

	header http.Header
	debug  bool

	ctx context.Context

	types.Logger
}

// Resp is the standard response envelope a gst backend answers with.
type Resp struct {
	Code    int             `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	TraceID string          `json:"trace_id,omitempty"`
	Cookies []*http.Cookie  `json:"-"`
}

// New creates a new client instance with given service base address and options.
// The address must start with "http://" or "https://". The client owns an
// isolated http.Client with its own cookie jar, so a login response cookie is
// carried on every later request automatically.
func New(addr string, opts ...Option) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the cookie jar")
	}
	client := &Client{
		httpClient: &http.Client{Jar: jar},
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
