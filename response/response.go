// Package response exposes the response entry points that code outside the
// framework's controller path needs: middleware, and the middleware a module
// ships to the projects that copy it.
//
// The envelope itself is not exposed. What a refusal looks like on the wire —
// which fields it carries, what is recorded alongside it — is the framework's
// to decide and to change, and a caller here states only the refusal it is
// making.
//
// This package forwards to the framework-internal implementation and adds no
// behavior of its own.
package response

import (
	"github.com/gin-gonic/gin"
	internalresponse "github.com/hydroan/gst/internal/response"
)

// Abort refuses the request with status and msg, written in the API envelope,
// and stops the handler chain. It is the one way to refuse a request from
// outside the controller path.
func Abort(c *gin.Context, status int, msg string) {
	internalresponse.Abort(c, status, msg)
}
