package modeliamuser

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserMarshalJSON(t *testing.T) {
	data, err := json.Marshal(&User{
		Username:     "marshal_user",
		Password:     "secret-password",
		PasswordHash: "hashed-password",
		Salt:         "salt-value",
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))

	require.Equal(t, "marshal_user", payload["username"])
	require.NotContains(t, payload, "password")
	require.NotContains(t, payload, "password_hash")
	require.NotContains(t, payload, "salt")
}
