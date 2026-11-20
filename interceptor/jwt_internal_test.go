package interceptor

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/authn/jwt"
	"github.com/stretchr/testify/require"
)

// TestSessionIDArrivesUnderTheSharedHeaderInLowerCase pins the metadata key
// of the session id: the header the HTTP middleware reads it from, already
// in lower case. Metadata lowercases a key on every read, and allocates to
// do it for a key that is not, twice a call here.
func TestSessionIDArrivesUnderTheSharedHeaderInLowerCase(t *testing.T) {
	require.Equal(t, strings.ToLower(jwt.HeaderSessionID), keySessionID)
}
