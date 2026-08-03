package client

import (
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/types"
)

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithHeader merges the given headers into the client defaults. A key present
// in header replaces the default value of that key; other defaults stay.
func WithHeader(header http.Header) Option {
	return func(c *Client) {
		for key, values := range header {
			c.header.Del(key)
			for _, value := range values {
				c.header.Add(key, value)
			}
		}
	}
}

func WithDebug() Option {
	return func(c *Client) {
		c.debug = true
	}
}

func WithLogger(logger types.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
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

func WithBasicAuth(username, password string) Option {
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
