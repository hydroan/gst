package testutil

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
)

func TestRequireErrorAcceptsWrappedRejection(t *testing.T) {
	err := errors.Wrap(&client.Error{
		StatusCode: http.StatusForbidden,
		Code:       403,
		Msg:        "permission denied for sample",
	}, "call sample endpoint")

	RequireError(t, err, http.StatusForbidden, "permission denied", "sample")
}
