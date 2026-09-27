package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
)

// actor names the caller in the X-Actor header. Registered with
// RegisterAuth, it runs behind the session check alone, where the caller is
// known; a public route never sees it.
func actor(c *gin.Context) {
	if username := c.GetString(consts.CTX_USERNAME); username != "" {
		c.Header("X-Actor", username)
	}
}
