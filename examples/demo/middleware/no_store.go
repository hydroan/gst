package middleware

import "github.com/gin-gonic/gin"

// noStore marks every API response as not to be cached: the API answers
// live rows, and a browser or proxy keeping a copy would serve stale ones.
// A middleware runs before the handler and simply returns; gin carries the
// chain on. One that refuses a request answers through response.Abort.
func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
}
