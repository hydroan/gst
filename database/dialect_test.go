package database_test

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

// TestDialectAppliesItsNamingLimits pins that the dialect's own settings
// reach the handle through the framework's wrapping of the driver: gorm
// hands the configuration to the driver before it fills in its defaults,
// and postgres limits an identifier to 63 characters there, where gorm's
// own default is 64; a name gorm checks by a longer limit than the database
// keeps is one it would never find again.
func TestDialectAppliesItsNamingLimits(t *testing.T) {
	naming, ok := database.DB().NamingStrategy.(schema.NamingStrategy)
	require.True(t, ok, "the handle runs on gorm's naming strategy: %T", database.DB().NamingStrategy)

	want := 64
	if config.App.Database.Type == config.DBPostgres {
		want = 63
	}
	require.Equal(t, want, naming.IdentifierMaxLength)
}
