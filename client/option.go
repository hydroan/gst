package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/types"
	"golang.org/x/time/rate"
)

type Option func(*Client)

func WithContext(ctx context.Context) Option {
	return func(c *Client) {
		if ctx != nil {
			c.ctx = ctx
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func WithHeader(header http.Header) Option {
	return func(c *Client) {
		if header != nil {
			c.header = header.Clone()
		}
	}
}

func WithDebug() Option {
	return func(c *Client) {
		c.debug = true
	}
}

func WithRetry(maxRetries int, wait time.Duration) Option {
	return func(c *Client) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		if wait < 0 {
			wait = 0
		}
		c.maxRetries = maxRetries
		c.retryWait = wait
	}
}

func WithRateLimit(r rate.Limit, b int) Option {
	return func(c *Client) {
		if r <= 0 || b <= 0 {
			return
		}
		c.rateLimiter = rate.NewLimiter(r, b)
	}
}

func WithLogger(logger types.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.Logger = logger
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout <= 0 {
			return
		}
		if c.httpClient == nil {
			c.httpClient = http.DefaultClient
		}
		c.httpClient.Timeout = timeout
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if c.header == nil {
			c.header = http.Header{}
		}
		c.header.Set("User-Agent", userAgent)
	}
}

// WithCookie adds a cookie to the client request headers.
func WithCookie(cookie *http.Cookie) Option {
	return func(c *Client) {
		if cookie == nil {
			return
		}
		if c.header == nil {
			c.header = http.Header{}
		}
		c.header.Add("Cookie", cookie.String())
	}
}

func WithBaseAuth(username, password string) Option {
	return func(c *Client) {
		if username = strings.TrimSpace(username); len(username) != 0 {
			c.username = username
			c.password = password
		}
	}
}

func WithToken(token string) Option {
	return func(c *Client) {
		if token = strings.TrimSpace(token); len(token) != 0 {
			c.token = token
		}
	}
}

// WithAPI sets a custom API path for the client.
// The path will be appended to the base address when making requests.
// Leading and trailing slashes are automatically handled.
func WithAPI(path string) Option {
	return func(c *Client) {
		if path = strings.TrimSpace(path); len(path) != 0 {
			c.apiPath = strings.Trim(path, "/")
		}
	}
}
