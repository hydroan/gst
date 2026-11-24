package client

import (
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/internal/gst"
)

type Option func(*Client)

// WithHTTPClient sends the requests through client instead of the http.Client
// New creates, whose cookie jar carries a login cookie over to later
// requests. The client is used as it is and never modified: a timeout set by
// WithTimeout applies to a copy.
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

// WithDebug has the client write every request it sends, headers and body
// included, and every response it reads to its logger at debug level (see
// WithLogger).
func WithDebug() Option {
	return func(c *Client) {
		c.debug = true
	}
}

// WithLogger sets the logger the output of WithDebug goes to; a nil logger
// keeps the client's own.
func WithLogger(logger gst.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithTimeout bounds each request of the client, the reading of the response
// included, by timeout (see http.Client.Timeout); a timeout of zero or less
// sets none. It applies alongside the deadline of the context a call passes:
// whichever ends first ends the request. New applies it last, to a copy of
// the http.Client, so the order of the options does not matter and a client
// handed to WithHTTPClient, such as a shared http.DefaultClient, keeps its own
// timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithUserAgent sets the User-Agent header of every request.
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

// WithBasicAuth has every request carry HTTP basic credentials; a username
// that is blank once trimmed sets none. A token set with WithToken takes
// their place: the Authorization header then carries the bearer token.
func WithBasicAuth(username, password string) Option {
	return func(c *Client) {
		if username = strings.TrimSpace(username); len(username) != 0 {
			c.username = username
			c.password = password
		}
	}
}

// WithToken has every request carry token as a bearer token in the
// Authorization header; a token that is blank once trimmed sets none.
func WithToken(token string) Option {
	return func(c *Client) {
		if token = strings.TrimSpace(token); len(token) != 0 {
			c.token = token
		}
	}
}
