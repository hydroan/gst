package modelauthz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBindingIDKeepsTheIDsOfStoredBindings pins the id bindingID gives three
// values: the stores of the projects key their bindings by it, so the same
// values must keep giving the same id.
func TestBindingIDKeepsTheIDsOfStoredBindings(t *testing.T) {
	require.Equal(t, "66b9ec68ea8183d132be601aa2a87eca", bindingID("user", "email", "address"))
	require.NotEqual(t, bindingID("user", "email", "address"), bindingID("address", "email", "user"))
}
