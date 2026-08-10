// Package response exposes the response entry points code outside the
// framework's controller path needs: middleware, and the middleware a module
// ships to the projects that copy it.
//
// The envelope itself is not exposed. What a refusal looks like on the wire —
// which fields it carries, what is recorded alongside it — is the framework's
// to decide and to change, and a caller here states only the refusal it is
// making.
package response

import (
	"github.com/gin-gonic/gin"
	internalresponse "github.com/hydroan/gst/internal/response"
)

// Abort refuses the request with status and msg, written in the API envelope,
// and stops the handler chain.
//
// It is the one way to refuse a request from outside the controller path.
// Writing the envelope by hand instead works until the envelope grows: the
// response code recorded for the body logger was added on the framework path
// and reached none of the hand-written ones, so every such refusal logged a
// code its own body contradicted.
func Abort(c *gin.Context, status int, msg string) {
	c.Abort()
	internalresponse.JSON(c, internalresponse.CodeFailure.WithStatus(status).WithMsg(msg))
}
