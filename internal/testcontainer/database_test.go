package testcontainer

import (
	"os"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupDatabase(t *testing.T) {
	t.Run("empty_type_falls_back_to_the_framework_default", func(t *testing.T) {
		isolateEnv(t, config.DATABASE_TYPE, config.SQLITE_PATH)

		release, err := SetupDatabase("")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, release()) })

		require.Equal(t, string(config.DBSqlite), os.Getenv(config.DATABASE_TYPE))
		require.NotEmpty(t, os.Getenv(config.SQLITE_PATH))
	})

	t.Run("unsupported_type_is_rejected", func(t *testing.T) {
		_, err := SetupDatabase(config.DBType("oracle"))

		require.Error(t, err)
		require.ErrorContains(t, err, "oracle")
	})
}
